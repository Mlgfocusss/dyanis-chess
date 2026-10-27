#!/bin/bash
# Final SPSA values (average of the second half, spsa-final.json) vs baseline.exe (= search-p8, old values), 100 ms, seeds 21 and 22.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 21 22; do
./match.exe -a ./search-spsa.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=spsafinal-carry.json >> spsafinal-match.log 2>&1
done
echo DONE >> spsafinal-match.log
