#!/bin/bash
# LMR remainder (cut node +1, TT capture +1, separate full-move counts) vs baseline.exe (corr + singular), 100 ms, seeds 5 and 6.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 5 6; do
./match.exe -a ./search-lmr.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=lmr-carry.json >> lmr-match.log 2>&1
done
echo DONE >> lmr-match.log
