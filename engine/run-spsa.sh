#!/bin/bash
# SPSA of the search parameters added/changed 2026-10-23 (NNUE, 100 ms). State: spsa-new.json, log: spsa-new.log
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
./spsa.exe -cli=tune.exe -match=match.exe -evalfile=net-kingf-m4.bin -iters=500 -pairs=16 -concurrency=16 -state=spsa-new.json \
  -params=NmpBase,NmpDivDepth,NmpEvalDiv,FutExtStep,LmpExtStep,HistLmrDiv,HistPruneMargin,CaptHistDiv,HistBonusMul,HistBonusCap,LmrBase,LmrDiv,Lmp3,Futility3 >> spsa-new.log 2>&1
echo DONE >> spsa-new.log
