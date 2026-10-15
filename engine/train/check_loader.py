"""Сверка загрузчика с эталоном: FEN, счёт, исход и флаги из `gendata -dump`.

    .venv\\Scripts\\python.exe train\\check_loader.py [каталог] [записей на шард]
"""
import os
import subprocess
import sys

import numpy as np
import torch

sys.path.insert(0, os.path.dirname(__file__))
import data  # noqa: E402

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GENDATA = os.path.join(ROOT, "gendata.exe")
LETTERS = "PNBRQK"


def to_fen(code, flags, cr_ep, halfmove, fullmove):
    rows = []
    for rank in range(7, -1, -1):
        s, empty = "", 0
        for f in range(8):
            c = int(code[rank * 8 + f])
            if c < 0:
                empty += 1
                continue
            if empty:
                s += str(empty)
                empty = 0
            ch = LETTERS[c & 7]
            s += ch.lower() if c >> 3 else ch
        if empty:
            s += str(empty)
        rows.append(s)
    cr = "".join(ch for bit, ch in ((1, "K"), (2, "Q"), (4, "k"), (8, "q")) if cr_ep & bit) or "-"
    ep_file = cr_ep >> 4
    black = flags & data.FLAG_BLACK
    ep = "-" if ep_file == 0 else "abcdefgh"[ep_file - 1] + ("3" if black else "6")
    return f"{'/'.join(rows)} {'b' if black else 'w'} {cr} {ep} {halfmove} {fullmove}"


def flag_names(flags):
    names = []
    if flags & data.FLAG_CHECK:
        names.append("check")
    if flags & data.FLAG_NOISY:
        names.append("noisy")
    if flags & data.FLAG_MATE:
        names.append("mate")
    return ",".join(names)


def main():
    directory = sys.argv[1] if len(sys.argv) > 1 else os.path.join(ROOT, "data", "nnue", "main1")
    n = int(sys.argv[2]) if len(sys.argv) > 2 else 300
    paths = data.shard_paths(directory)
    picks = [paths[0], paths[len(paths) // 2], paths[-1]]
    results = {"1-0": 2, "1/2": 1, "0-1": 0}
    total = bad = 0
    for p in picks:
        out = subprocess.run([GENDATA, "-dump", p, "-n", str(n)], capture_output=True, text=True,
                             encoding="utf-8").stdout.strip().splitlines()
        raw = torch.from_numpy(np.fromfile(p, dtype=np.uint8).reshape(-1, 32)[: len(out)])
        code = data.decode_pieces(raw)
        score, result, flags, cr_ep, hm, fm = data.decode_fields(raw)
        for i, line in enumerate(out):
            fen, sc, res, fl = [x.strip() for x in line.split("|")]
            mine = (to_fen(code[i].tolist(), int(flags[i]), int(cr_ep[i]), int(hm[i]), int(fm[i])),
                    int(score[i]), int(result[i]), flag_names(int(flags[i])))
            ref = (fen, int(sc), results[res], fl)
            total += 1
            if mine != ref:
                bad += 1
                if bad <= 5:
                    print("MISMATCH", os.path.basename(p), i, "\n  mine:", mine, "\n  ref: ", ref)
    print(f"сверено {total} записей, расхождений {bad}")

    # признаки: у каждой записи ровно столько активных, сколько фигур
    raw = data.load_raw(directory, max_shards=20)
    keep = data.keep_mask(raw)
    print(f"20 шардов: {len(raw)} записей, после отбора {int(keep.sum())} ({keep.float().mean():.1%})")
    us, them, tgt, s, _ = data.batch(raw[:100000][keep[:100000]])
    pieces = (data.decode_pieces(raw[:100000][keep[:100000]]) >= 0).sum(1)
    assert ((us != data.layout("none").pad).sum(1) == pieces).all() and ((them != data.layout("none").pad).sum(1) == pieces).all()
    assert us[us != data.layout("none").pad].max() < data.N_FEATURES
    print(f"признаки: активных {pieces.float().mean():.1f} в среднем, цель {tgt.min():.3f}..{tgt.max():.3f}")
    if torch.cuda.is_available():
        g = data.batch(raw[:100000][keep[:100000]].cuda())
        assert torch.equal(g[0].cpu(), us) and torch.equal(g[1].cpu(), them)
        print("GPU и CPU дают одинаковые признаки")
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
