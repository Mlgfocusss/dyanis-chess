#!/bin/bash
# Correction history + singular extensions (search-sing.exe) vs baseline.exe (packages 1-3), 100 ms, seeds 3 and 4.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 3 4; do
./match.exe -a ./search-sing.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=both-carry.json >> both-match.log 2>&1
done
echo DONE >> both-match.log
