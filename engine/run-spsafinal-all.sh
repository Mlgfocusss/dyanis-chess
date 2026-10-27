#!/bin/bash
# All 22 final SPSA values set on baseline.exe via setoption (A) vs plain baseline.exe (B), 100 ms, seed 21, 100 pairs.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
B="EvalFile=net-kingf-m4.bin,Hash=16"
ALL="CaptHistDiv=4,CorrNonPawnW=2906,CorrPawnW=6623,EvalMatBase=45,EvalR50Base=224,FutExtStep=130,Futility3=601,HistBonusCap=1758,HistBonusMul=8,HistLmrDiv=4226,HistPruneMargin=2292,Lmp3=17,LmpExtStep=7,LmrBase=78,LmrDiv=198,LmrFullNonPV=3,LmrFullPV=6,NmpBase=3,NmpDivDepth=4,NmpEvalDiv=195,SeeQuietMul=30,SingMargin=21"
echo "### ALL 22 via setoption on baseline.exe" >> spsafinal-groups.log
./match.exe -a ./baseline.exe -b ./baseline.exe -aopts "$B,$ALL" -bopts "$B" -movetime=100 -pairs=100 -concurrency=16 -openbook=assets/gm2001.bin -seed=21 -carry=spsafinal-gALL-carry.json 2>&1 | grep -v "^game" >> spsafinal-groups.log
echo DONE2 >> spsafinal-groups.log
