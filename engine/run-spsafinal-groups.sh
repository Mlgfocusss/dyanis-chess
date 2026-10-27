#!/bin/bash
# Bisect of the final SPSA values: each group set on baseline.exe via setoption (A) vs plain baseline.exe (B), 100 ms, seed 21, 100 pairs.
cd /c/Users/Dyanis/Desktop/dyanis-chess-engine/engine
B="EvalFile=net-kingf-m4.bin,Hash=16"
run(){ name=$1; opts=$2
echo "### group $name: $opts" >> spsafinal-groups.log
./match.exe -a ./baseline.exe -b ./baseline.exe -aopts "$B,$opts" -bopts "$B" -movetime=100 -pairs=100 -concurrency=16 -openbook=assets/gm2001.bin -seed=21 -carry=spsafinal-g$name-carry.json 2>&1 | grep -v "^game" >> spsafinal-groups.log
}
run EVAL "EvalMatBase=45,EvalR50Base=224,CorrPawnW=6623,CorrNonPawnW=2906"
run PRUNE "NmpDivDepth=4,NmpEvalDiv=195,FutExtStep=130,LmpExtStep=7,Lmp3=17,Futility3=601"
run LMR "LmrBase=78,LmrDiv=198,LmrFullPV=6,SingMargin=21,HistBonusMul=8,HistBonusCap=1758,HistLmrDiv=4226,HistPruneMargin=2292"
echo DONE >> spsafinal-groups.log
