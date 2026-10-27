#!/bin/bash
# Singular extensions (search-sing.exe) vs search-corr.exe (same code with SwSing available), 100 ms, 2 x 300 pairs.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
for s in 1 2; do
./match.exe -a ./search-sing.exe -b ./search-corr.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bopts "EvalFile=net-kingf-m4.bin,Hash=16" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=sing-carry.json >> sing-match.log 2>&1
done
echo DONE >> sing-match.log
