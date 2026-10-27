#!/bin/bash
# bench.sh exe [depth] : fixed-depth search (no book) on four positions, waits for bestmove; prints total nodes, wall ms, nps.
exe=$1; depth=${2:-15}
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
tn=0; tt=0
for fen in "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1" \
  "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1" \
  "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1" \
  "r1bq1rk1/pp2ppbp/2np1np1/8/3NP3/2N1BP2/PPPQ2PP/R3KB1R w KQ - 0 9"; do
  f=$(mktemp)
  s=$(date +%s%N)
  { printf "setoption name EvalFile value net-kingf-m4.bin\nsetoption name Hash value 16\nposition fen $fen\ngo depth $depth\n"; until grep -q bestmove $f; do sleep 0.05; done; echo quit; } | $exe -uci -book= > $f 2>&1
  e=$(date +%s%N)
  n=$(grep "^info depth" $f | tail -1 | sed -n 's/.* nodes \([0-9]*\).*/\1/p'); rm -f $f
  tn=$((tn+n)); tt=$((tt+(e-s)/1000000))
done
echo "$exe depth=$depth nodes=$tn ms=$tt nps=$((tn*1000/tt))"
