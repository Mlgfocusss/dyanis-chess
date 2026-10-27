#!/bin/bash
# Item 6 (eval scaled by material and fifty-move counter) vs baseline.exe (= p5: corr + singular + items 4, 5), 100 ms, seeds 9 and 10.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 9 10; do
./match.exe -a ./search-p6.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=p6-carry.json >> p6-match.log 2>&1
done
echo DONE >> p6-match.log
