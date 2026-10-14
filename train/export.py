"""Экспорт чекпойнта в файл весов DYNN (NNUE.md) и сверка с Go.

    .venv\\Scripts\\python.exe train\\export.py train\\out\\h256\\checkpoint.pt net.bin [--verify 2000]

Сверка: целочисленный эталон по формуле из NNUE.md считает оценки первых N
записей шарда пилота, `nnuetool eval` — те же записи; значения должны совпасть
до единицы (со стороны белых).
"""
import argparse
import os
import struct
import subprocess
import sys
import zlib

import numpy as np
import torch

sys.path.insert(0, os.path.dirname(__file__))
import data  # noqa: E402
from train import Net, QA, QB, SCALE  # noqa: E402

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def quantize(net):
    h = net.b1.numel()
    w1f = net.w1().detach().cpu().numpy()
    if np.abs(w1f).max() * QA >= 32767:
        print(f"предупреждение: {(np.abs(w1f) * QA >= 32767).sum()} весов W1 обрезано до int16")
    w1 = np.clip(np.round(w1f * QA), -32767, 32767).astype(np.int64)
    b1 = np.round(net.b1.detach().cpu().numpy() * QA).astype(np.int64)
    w2 = np.round(net.out_w.detach().cpu().numpy() * QB).astype(np.int64)            # (корзины, 2H)
    b2 = np.round(net.out_b.detach().cpu().numpy() * QA * QB).astype(np.int64)         # (корзины,)
    assert w1.shape == (net.lay.n_features, h) and w2.shape == (net.buckets, 2 * h)
    assert np.abs(w1).max() < 32768 and np.abs(b1).max() < 32768 and np.abs(w2).max() <= 127
    return w1, b1, w2, b2


def write_dynn(path, net, act):
    w1, b1, w2, b2 = quantize(net)
    h = b1.size
    lay = net.lay
    v3 = lay.K > 1 or lay.mirror
    blob = b"DYNN" + struct.pack("<IIIIiiii", 3 if v3 else 2, data.N_FEATURES, h, 0 if act == "crelu" else 1,
                                 QA, QB, int(SCALE), w2.shape[0])
    if v3:
        blob += struct.pack("<II", lay.K, 1 if lay.mirror else 0) + bytes(lay.king_map)
    blob += w1.astype("<i2").tobytes() + b1.astype("<i2").tobytes() + w2.astype("<i2").tobytes()
    blob += b2.astype("<i4").tobytes()
    blob += struct.pack("<I", zlib.crc32(blob) & 0xFFFFFFFF)
    with open(path, "wb") as f:
        f.write(blob)
    return w1, b1, w2, b2


def trunc_div(a, b):
    """Целочисленное деление с усечением к нулю (как в Go)."""
    q = np.abs(a) // b
    return np.where(a < 0, -q, q)


def reference_eval(raw, act, w1, b1, w2, b2, lay=None):
    """Эталон на целых числах: оценка со стороны белых для (N, 32) записей.
    w2 — (корзины, 2H), b2 — (корзины,)."""
    code = data.decode_pieces(raw)
    flags = raw[:, 27].long()
    us, them = data.features(code, flags & data.FLAG_BLACK, lay)
    w1p = np.vstack([w1, np.zeros((1, w1.shape[1]), dtype=np.int64)])   # строка PAD — нули

    def acc(idx):
        return w1p[idx.numpy()].sum(1) + b1

    def activate(a):
        a = np.clip(a, 0, QA)
        return a * a if act == "screlu" else a

    h = b1.size
    pieces = (code >= 0).sum(1).numpy()
    k = np.minimum(np.maximum(pieces - 2, 0) // 4, w2.shape[0] - 1)
    w = w2[k]
    out = (activate(acc(us)) * w[:, :h]).sum(1) + (activate(acc(them)) * w[:, h:]).sum(1)
    if act == "screlu":
        out = trunc_div(out, QA)
    ev = trunc_div((out + b2[k]) * int(SCALE), QA * QB)
    black = (flags & data.FLAG_BLACK).numpy().astype(bool)
    return np.where(black, -ev, ev)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("checkpoint")
    ap.add_argument("out")
    ap.add_argument("--verify", type=int, default=2000)
    args = ap.parse_args()

    ck = torch.load(args.checkpoint, map_location="cpu")
    a = ck["args"]
    net = Net(a["hidden"], a["act"], a.get("buckets", 8), data.layout(a.get("layout", "none")), a.get("factor", False))
    net.load_state_dict(ck["state"])
    net.eval()
    w1, b1, w2, b2 = write_dynn(args.out, net, a["act"])
    print(f"записано {args.out}: H={a['hidden']} {a['act']} корзин {net.buckets}, король {net.lay.name} ({net.lay.K}), эпоха {ck['epoch']}, {os.path.getsize(args.out)} байт")

    if args.verify:
        shard = data.shard_paths(os.path.join(ROOT, "data", "nnue", "pilot"))[3]
        n = args.verify
        raw = torch.from_numpy(np.fromfile(shard, dtype=np.uint8).reshape(-1, 32)[:n])
        n = len(raw)
        ref = reference_eval(raw, a["act"], w1, b1, w2, b2, net.lay)
        res = subprocess.run([os.path.join(ROOT, "nnuetool.exe"), "eval", args.out, shard, "-n", str(n)],
                             capture_output=True, text=True, encoding="utf-8")
        go = np.array([int(l.rsplit("|", 1)[1]) for l in res.stdout.strip().splitlines()])
        if len(go) != n:
            sys.exit(f"nnuetool вернул {len(go)} строк вместо {n}: {res.stderr[:300]}")
        diff = np.abs(go - ref)
        print(f"Go и Python-эталон на {n} записях: расхождений {int((diff > 0).sum())}, макс. {diff.max()}")
        with torch.no_grad():
            us, them = data.features(data.decode_pieces(raw), raw[:, 27].long() & data.FLAG_BLACK, net.lay)
            f = net(us, them, (data.decode_pieces(raw) >= 0).sum(1)).numpy() * SCALE
        black = (raw[:, 27].long() & data.FLAG_BLACK).numpy().astype(bool)
        f = np.where(black, -f, f)
        print(f"float-модель против целочисленной: средняя |разница| {np.abs(f - go).mean():.2f} сп, макс. {np.abs(f - go).max():.1f}")
        sys.exit(1 if diff.max() > 0 else 0)


if __name__ == "__main__":
    main()
