#!/bin/bash
# Re-check of final SPSA values on AC power (Turbo plan): search-spsa.exe vs baseline.exe, 100 ms, seed 21, 300 pairs.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
./match.exe -a ./search-spsa.exe -b ./baseline.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=21 -carry=spsafinal-ac-carry.json >> spsafinal-ac-match.log 2>&1
echo DONE >> spsafinal-ac-match.log
