#!/bin/bash
# Final SPSA values (set via setoption on the SIMD build baseline.exe) vs plain baseline.exe, 100 ms, seeds 21 and 22, 300 pairs each.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
B="EvalFile=net-kingf-m4.bin,Hash=16"
ALL="CaptHistDiv=4,CorrNonPawnW=2906,CorrPawnW=6623,EvalMatBase=45,EvalR50Base=224,FutExtStep=130,Futility3=601,HistBonusCap=1758,HistBonusMul=8,HistLmrDiv=4226,HistPruneMargin=2292,Lmp3=17,LmpExtStep=7,LmrBase=78,LmrDiv=198,LmrFullNonPV=3,LmrFullPV=6,NmpBase=3,NmpDivDepth=4,NmpEvalDiv=195,SeeQuietMul=30,SingMargin=21"
for s in 21 22; do
./match.exe -a ./baseline.exe -b ./baseline.exe -aopts "$B,$ALL" -bopts "$B" -movetime=100 -pairs=300 -concurrency=16 -openbook=assets/gm2001.bin -seed=$s -carry=spsafinal-simd-carry.json >> spsafinal-simd-match.log 2>&1
done
echo DONE >> spsafinal-simd-match.log
