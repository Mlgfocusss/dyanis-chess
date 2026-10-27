#!/bin/bash
# SPSA of 22 search/eval parameters on the build of 2026-10-24 (search-p8 code), NNUE, 100 ms. State: spsa-final.json, log: spsa-final.log (rerun the script to resume).
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
./spsa.exe -cli=tune.exe -match=match.exe -evalfile=net-kingf-m4.bin -iters=500 -pairs=16 -concurrency=16 -state=spsa-final.json \
  -params=NmpBase,NmpDivDepth,NmpEvalDiv,FutExtStep,LmpExtStep,HistLmrDiv,HistPruneMargin,CaptHistDiv,HistBonusMul,HistBonusCap,LmrBase,LmrDiv,Lmp3,Futility3,SingMargin,LmrFullPV,LmrFullNonPV,SeeQuietMul,EvalMatBase,EvalR50Base,CorrPawnW,CorrNonPawnW >> spsa-final.log 2>&1
echo DONE >> spsa-final.log
