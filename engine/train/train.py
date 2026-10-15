"""Обучение сети 768 -> H -> 1 (две перспективы) для internal/nnue.

    .venv\\Scripts\\python.exe train\\train.py --epochs 1 --max-shards 100 --out train\\out\\smoke
    .venv\\Scripts\\python.exe train\\train.py --epochs 20 --out train\\out\\h256

Выходной слой выбирается по числу фигур на доске (корзины, NNUE.md).
Модель в «float-единицах»: активация аккумулятора a = clamp(acc, 0, 1) (CReLU)
или a² (SCReLU), выход o = W2·[a_us, a_them] + b2, оценка в сантипешках = o*SCALE.
Квантование (QA, QB) делает экспорт. Веса W1/B1/W2 обрезаются после каждого
шага под ограничения формата DYNN (NNUE.md).
"""
import argparse
import json
import math
import os
import sys
import time

import numpy as np
import torch
import torch.nn as nn

sys.path.insert(0, os.path.dirname(__file__))
import data  # noqa: E402

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCALE = 400.0
QA, QB = 255, 64
CLIP_W1 = 127.0 * 4 / QA      # ±1.98: int16-аккумулятор не переполняется (32 строки W1)
CLIP_W2 = 127.0 / QB          # ±1.98: |W2| <= 127 в квантованном виде


class Net(nn.Module):
    def __init__(self, hidden, act, buckets=8, lay=None, factor=False):
        super().__init__()
        self.act = act
        self.buckets = buckets
        self.lay = lay or data.layout("none")
        self.factor = factor and self.lay.K > 1
        self.ft = nn.EmbeddingBag(self.lay.n_features + 1, hidden, mode="sum", padding_idx=self.lay.pad)
        if self.factor:
            # общий для всех корзин король-независимый вес: вес признака = общий + свой для корзины
            self.fs = nn.EmbeddingBag(data.N_FEATURES + 1, hidden, mode="sum", padding_idx=data.N_FEATURES)
        self.b1 = nn.Parameter(torch.zeros(hidden))
        self.out_w = nn.Parameter(torch.zeros(buckets, 2 * hidden))
        self.out_b = nn.Parameter(torch.zeros(buckets))
        with torch.no_grad():
            self.ft.weight.normal_(0, 0.0 if self.factor else 0.05)
            self.ft.weight[self.lay.pad].zero_()
            if self.factor:
                self.fs.weight.normal_(0, 0.05)
                self.fs.weight[data.N_FEATURES].zero_()
            self.out_w.normal_(0, 0.05)

    def activate(self, x):
        x = x.clamp(0, 1)
        return x * x if self.act == "screlu" else x

    def features(self, idx):
        x = self.ft(idx)
        if self.factor:
            x = x + self.fs(torch.where(idx == self.lay.pad, data.N_FEATURES, idx % data.N_FEATURES))
        return x

    def w1(self):
        """Итоговые веса первого слоя (n_features, H): с общей частью, если она есть."""
        w = self.ft.weight[: self.lay.n_features]
        if self.factor:
            w = w + self.fs.weight[: data.N_FEATURES].repeat(self.lay.K, 1)
        return w

    def bucket(self, pieces):
        """Выходная корзина по числу фигур: (pieces - 2) // 4, не выше последней (как Network.Bucket в Go)."""
        return ((pieces - 2).clamp(min=0) // 4).clamp(max=self.buckets - 1)

    def forward(self, us, them, pieces):
        a = self.activate(self.features(us) + self.b1)
        b = self.activate(self.features(them) + self.b1)
        k = self.bucket(pieces)
        return (torch.cat((a, b), 1) * self.out_w[k]).sum(1) + self.out_b[k]

    @torch.no_grad()
    def clip(self):
        self.ft.weight.clamp_(-CLIP_W1, CLIP_W1)
        self.ft.weight[self.lay.pad].zero_()
        if self.factor:
            self.fs.weight.clamp_(-CLIP_W1, CLIP_W1)
            self.fs.weight[data.N_FEATURES].zero_()
        self.b1.clamp_(-CLIP_W1, CLIP_W1)
        self.out_w.clamp_(-CLIP_W2, CLIP_W2)


def load_set(directory, max_shards, device):
    raw = data.load_raw(directory, max_shards)
    keep = data.keep_mask(raw)
    return raw, keep


def evaluate(net, raw, keep, args, device, bs=65536):
    """MSE сети и (если есть файлы *.eval.i16) ручного эвала на отобранных записях."""
    net.eval()
    idx = keep.nonzero().squeeze(1)
    tot = 0.0
    with torch.no_grad():
        for i in range(0, len(idx), bs):
            r = raw[idx[i:i + bs]].to(device)
            us, them, tgt, _, pc = data.batch(r, args.k, args.lam, net.lay)
            pred = torch.sigmoid(net(us, them, pc) * SCALE / args.k)
            tot += ((pred - tgt) ** 2).sum().item()
    net.train()
    return tot / len(idx)


def handcrafted_mse(directory, max_shards, raw, keep, args, device):
    paths = data.shard_paths(directory)[:max_shards]
    files = [p[:-4] + ".eval.i16" for p in paths]
    if not all(os.path.exists(f) for f in files):
        return None
    ev = np.concatenate([np.fromfile(f, dtype="<i2") for f in files])
    if len(ev) != len(raw):
        return None
    ev = torch.from_numpy(ev.astype(np.float32))
    idx = keep.nonzero().squeeze(1)
    r = raw[idx].to(device)
    _, _, tgt, _, _ = data.batch(r, args.k, args.lam)
    black = (r[:, 27].long() & data.FLAG_BLACK).bool()
    e = ev[idx].to(device)
    e = torch.where(black, -e, e)       # Evaluate — со стороны белых -> со стороны хода
    pred = torch.sigmoid(e / args.k)
    return ((pred - tgt) ** 2).mean().item()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default=os.path.join(ROOT, "data", "nnue", "main1"))
    ap.add_argument("--val", default=os.path.join(ROOT, "data", "nnue", "pilot"))
    ap.add_argument("--max-shards", type=int, default=None)
    ap.add_argument("--hidden", type=int, default=256)
    ap.add_argument("--buckets", type=int, default=8, help="выходных корзин по числу фигур (1 — без корзин)")
    ap.add_argument("--layout", default="none", choices=sorted(data.LAYOUTS),
                    help="раскладка входов по положению своего короля (none — 768 входов)")
    ap.add_argument("--factor", action="store_true",
                    help="общая часть весов для всех корзин короля (меньше переобучение; при экспорте суммируется)")
    ap.add_argument("--act", choices=("crelu", "screlu"), default="screlu")
    ap.add_argument("--epochs", type=int, default=10)
    ap.add_argument("--batch", type=int, default=16384)
    ap.add_argument("--lr", type=float, default=1e-3)
    ap.add_argument("--lr-final", type=float, default=1e-5)
    ap.add_argument("--k", type=float, default=400.0)
    ap.add_argument("--lam", type=float, default=0.75)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--out", default=os.path.join(ROOT, "train", "out", "run"))
    args = ap.parse_args()

    torch.manual_seed(args.seed)
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    torch.backends.cuda.matmul.allow_tf32 = True
    os.makedirs(args.out, exist_ok=True)

    t0 = time.time()
    raw, keep = load_set(args.data, args.max_shards, device)
    vraw, vkeep = load_set(args.val, None, device)
    raw = raw.to(device)
    train_idx = keep.nonzero().squeeze(1).to(device)
    n = len(train_idx)
    print(f"обучение: {len(raw)} записей, после отбора {n}; валидация: {int(vkeep.sum())}; "
          f"загрузка {time.time() - t0:.0f} с, устройство {device}", flush=True)
    hc = handcrafted_mse(args.val, None, vraw, vkeep, args, device)
    if hc is not None:
        print(f"ручной эвал на валидации: MSE {hc:.6f}", flush=True)

    net = Net(args.hidden, args.act, args.buckets, data.layout(args.layout), args.factor).to(device)
    opt = torch.optim.Adam(net.parameters(), lr=args.lr)
    steps_per_epoch = n // args.batch
    total = steps_per_epoch * args.epochs
    step = 0
    log = []
    for epoch in range(1, args.epochs + 1):
        perm = train_idx[torch.randperm(n, device=device)]
        run, cnt, te = 0.0, 0, time.time()
        for i in range(steps_per_epoch):
            lr = args.lr_final + 0.5 * (args.lr - args.lr_final) * (1 + math.cos(math.pi * step / total))
            for g in opt.param_groups:
                g["lr"] = lr
            us, them, tgt, _, pc = data.batch(raw[perm[i * args.batch:(i + 1) * args.batch]], args.k, args.lam, net.lay)
            pred = torch.sigmoid(net(us, them, pc) * SCALE / args.k)
            loss = ((pred - tgt) ** 2).mean()
            opt.zero_grad(set_to_none=True)
            loss.backward()
            opt.step()
            net.clip()
            step += 1
            run += loss.item() if i % 20 == 0 else 0.0
            cnt += 1 if i % 20 == 0 else 0
            if (i + 1) % 100 == 0:
                print(f"  эпоха {epoch} шаг {i + 1}/{steps_per_epoch} loss {run / cnt:.6f} lr {lr:.2e}", flush=True)
        val = evaluate(net, vraw, vkeep, args, device)
        rec = {"epoch": epoch, "train_loss": run / cnt, "val_mse": val, "handcrafted_mse": hc,
               "seconds": time.time() - te}
        log.append(rec)
        print(json.dumps(rec), flush=True)
        torch.save({"state": net.state_dict(), "args": vars(args), "epoch": epoch},
                   os.path.join(args.out, "checkpoint.pt"))
        if val <= min(r["val_mse"] for r in log):
            torch.save({"state": net.state_dict(), "args": vars(args), "epoch": epoch},
                       os.path.join(args.out, "best.pt"))
        with open(os.path.join(args.out, "log.json"), "w") as f:
            json.dump(log, f, indent=1)
    print("готово", flush=True)


if __name__ == "__main__":
    main()
