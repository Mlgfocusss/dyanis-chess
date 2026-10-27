#!/bin/bash
# Item 5 (futility by lmrDepth, quiet SEE pruning) vs search-lmr.exe (item 4 included), 100 ms, seeds 7 and 8.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 7 8; do
./match.exe -a ./search-p5.exe -b ./search-lmr.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=p5-carry.json >> p5-match.log 2>&1
done
echo DONE >> p5-match.log
