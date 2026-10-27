#!/bin/bash
# Correction history (search-corr.exe) vs baseline.exe (packages 1-3), 100 ms, 2 series x 300 pairs, no SPSA.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 1 2; do
./match.exe -a ./search-corr.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=corr-carry.json >> corr-match.log 2>&1
done
echo DONE >> corr-match.log
