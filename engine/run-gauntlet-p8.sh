#!/bin/bash
# Stockfish 19 gauntlet at UCI_Elo 3000, 500 ms, 1 thread, build search-p8.exe (all of 2026-10-24). Same seed as the reference run.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
./match.exe -a ./search-p8.exe -b engines/sf19/stockfish/stockfish-windows-x86-64-universal.exe -aopts "EvalFile=net-kingf-m4.bin,Hash=16" -bargs= -bopts "UCI_LimitStrength=true,UCI_Elo=3000,Threads=1" -movetime=500 -pairs=100 -concurrency=8 -openbook=assets/gm2001.bin -seed=11 -carry=gauntlet-3000-p8-carry.json >> gauntlet-3000-p8.log 2>&1
echo DONE >> gauntlet-3000-p8.log
