#!/bin/bash
# New SPSA values but NmpDivDepth=3 (A) vs baseline.exe (B), 100 ms, seed 21, 300 pairs.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
./match.exe -a ./search-spsa.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16,NmpDivDepth=3" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=21 -carry=spsafinal-nmp3-carry.json >> spsafinal-nmp3-match.log 2>&1
echo DONE >> spsafinal-nmp3-match.log
