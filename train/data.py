"""Загрузчик обучающих данных NNUE (формат записи — NNUE.md, 32 байта).

Все шарды читаются в один тензор uint8 (N, 32); декодирование в позицию и
признаки идёт пакетами на любом устройстве (CPU/GPU) средствами torch.
"""
import glob
import os

import numpy as np
import torch

RECORD_BYTES = 32
N_FEATURES = 768        # признаков на одну корзину короля
MAX_PIECES = 32


class Layout:
    """Раскладка входов по положению своего короля (NNUE.md, «Корзины короля»).
    king_map[sq] — корзина клетки короля, как её видит перспектива (для чёрных
    с отражением по вертикали; при mirror — ещё и с отражением по горизонтали,
    так что используются только файлы a–d). Номера корзин плотные: 0..K-1."""

    def __init__(self, name, mirror, king_map):
        self.name = name
        self.mirror = mirror
        self.king_map = list(king_map)
        self.K = max(self.king_map) + 1
        self.n_features = N_FEATURES * self.K
        self.pad = self.n_features      # индекс "пустого" признака (нулевая строка в EmbeddingBag)


def _layout(name):
    if name == "none":
        return Layout(name, False, [0] * 64)
    m = [0] * 64
    for sq in range(64):
        f, r = sq & 7, sq >> 3          # f — файл после отражения (0..3 значимы), r — горизонталь с точки зрения перспективы
        if name == "m4":                # 1-я горизонталь a–b / c–d, 2-я–3-я, остальное
            m[sq] = (0 if f < 2 else 1) if r == 0 else (2 if r <= 2 else 3)
        elif name == "m9":
            if r == 0:
                m[sq] = min(f, 3)       # a b c d; рокированный король (g1/h1) после отражения — это b/a
            elif r == 1:
                m[sq] = 4 if f < 2 else 5
            else:
                m[sq] = 6 if r == 2 else 7 if r == 3 else 8
        elif name == "m16":             # три нижние горизонтали по файлам, выше — 4 корзины по файлам
            m[sq] = r * 4 + min(f, 3) if r < 3 else 12 + min(f, 3)
        else:
            raise ValueError(f"неизвестная раскладка {name}")
    return Layout(name, True, m)


LAYOUTS = {n: _layout(n) for n in ("none", "m4", "m9", "m16")}


def layout(name):
    return LAYOUTS[name]

FLAG_BLACK = 1          # бит 0: ходят чёрные
FLAG_CHECK = 2          # бит 1: шах
FLAG_NOISY = 4          # бит 2: лучший ход шумный
FLAG_MATE = 8           # бит 3: матовый счёт


def shard_paths(directory):
    """Целые шарды: рядом с shard-N.bin лежит shard-N.json (пишется последним)."""
    out = []
    for p in sorted(glob.glob(os.path.join(directory, "shard-*.bin"))):
        if os.path.exists(p[:-4] + ".json"):
            out.append(p)
    return out


def load_raw(directory, max_shards=None):
    """Все записи каталога (или нескольких через запятую) как uint8-тензор (N, 32)."""
    paths = []
    for d in directory.split(","):
        part = shard_paths(d)
        paths += part[:max_shards] if max_shards is not None else part
    parts = []
    for p in paths:
        a = np.fromfile(p, dtype=np.uint8)
        if a.size % RECORD_BYTES:
            raise ValueError(f"{p}: размер {a.size} не кратен {RECORD_BYTES}")
        parts.append(a.reshape(-1, RECORD_BYTES))
    if not parts:
        raise FileNotFoundError(f"нет целых шардов в {directory}")
    return torch.from_numpy(np.concatenate(parts))


def keep_mask(raw):
    """Отбор: без шаха, без шумного лучшего хода, без мата."""
    flags = raw[:, 27]
    return (flags & (FLAG_CHECK | FLAG_NOISY | FLAG_MATE)) == 0


def decode_pieces(raw):
    """(N, 32) uint8 -> коды фигур по клеткам (N, 64) int64, -1 — пусто.
    Код: цвет<<3 | (тип-1), т. е. 0..5 белые P N B R Q K, 8..13 чёрные."""
    r = raw.long()
    n = r.shape[0]
    shifts = torch.arange(8, device=r.device)
    occ = ((r[:, :8, None] >> shifts) & 1).reshape(n, 64)   # бит i = клетка i
    k = occ.cumsum(1) - 1                                    # номер фигуры по клетке
    p = r[:, 8:24]
    nib = torch.stack((p & 15, p >> 4), -1).reshape(n, 32)   # чётный k — младший полубайт
    code = nib.gather(1, k.clamp(min=0))
    return torch.where(occ.bool(), code, torch.full_like(code, -1))


def decode_fields(raw):
    """score (со стороны белых), result (0/1/2 со стороны белых), flags,
    castling+ep byte, halfmove, fullmove."""
    r = raw.long()
    score = r[:, 24] | (r[:, 25] << 8)
    score = torch.where(score >= 32768, score - 65536, score)
    fullmove = r[:, 30] | (r[:, 31] << 8)
    return score, r[:, 26], r[:, 27], r[:, 28], r[:, 29], fullmove


def features(code, black_to_move, lay=None):
    """Индексы признаков (N, 32) для стороны на ходу и для соперника.
    index = bucket*768 + (relColor*6 + (type-1))*64 + square; для перспективы
    чёрных square ^ 56; relColor 0 — фигура той же стороны, что и перспектива;
    bucket — по клетке своего короля (lay.king_map), при lay.mirror ещё и
    square ^ 7, если король на файлах e–h."""
    lay = lay or LAYOUTS["none"]
    valid = code >= 0
    color = code >> 3
    typ = code & 7
    sq = torch.arange(64, device=code.device).expand_as(code)
    kmap = torch.tensor(lay.king_map, device=code.device)

    def side(king_code, flip_rank):
        ks = (code == king_code).long().argmax(1)
        if flip_rank:
            ks = ks ^ 56
        flip = ((ks & 7) >= 4).long() * 7 if lay.mirror else torch.zeros_like(ks)
        return kmap[ks ^ flip][:, None] * N_FEATURES, flip[:, None]

    bw, fw = side(5, False)
    bb, fb = side(13, True)
    idx_w = bw + (color * 6 + typ) * 64 + (sq ^ fw)
    idx_b = bb + ((1 - color) * 6 + typ) * 64 + ((sq ^ 56) ^ fb)
    pad = torch.full_like(idx_w, lay.pad)
    idx_w = torch.where(valid, idx_w, pad)
    idx_b = torch.where(valid, idx_b, pad)
    stm = black_to_move.bool()[:, None]
    us = torch.where(stm, idx_b, idx_w)
    them = torch.where(stm, idx_w, idx_b)
    us = us.sort(1).values[:, :MAX_PIECES]      # PAD — наибольший индекс, уходит в хвост
    them = them.sort(1).values[:, :MAX_PIECES]
    return us, them


def targets(raw, k=400.0, lam=0.75):
    """Цель со стороны того, кто ходит: lam*sigmoid(score/k) + (1-lam)*исход."""
    score, result, flags, _, _, _ = decode_fields(raw)
    black = (flags & FLAG_BLACK).bool()
    s = torch.where(black, -score, score).float()
    wdl = result.float() / 2
    wdl = torch.where(black, 1 - wdl, wdl)
    return lam * torch.sigmoid(s / k) + (1 - lam) * wdl, s


def batch(raw, k=400.0, lam=0.75, lay=None):
    """Всё нужное обучению для пакета записей: (us, them, target, score_stm,
    число фигур на доске — по нему выбирается выходная корзина)."""
    code = decode_pieces(raw)
    flags = raw[:, 27].long()
    us, them = features(code, flags & FLAG_BLACK, lay)
    tgt, s = targets(raw, k, lam)
    return us, them, tgt, s, (code >= 0).sum(1)
