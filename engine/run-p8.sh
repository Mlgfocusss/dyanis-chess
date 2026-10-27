#!/bin/bash
# Item 8 (qsearch fail-soft, evasion sorting, quiet queen promotions) + item 7 speedups vs baseline.exe (= p6), 100 ms, seeds 11 and 12.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 11 12; do
./match.exe -a ./search-p8.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=p8-carry.json >> p8-match.log 2>&1
done
echo DONE >> p8-match.log
