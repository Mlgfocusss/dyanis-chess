// Command nnuetool inspects and produces NNUE weight files (format in
// NNUE.md).
//
//	nnuetool info net.bin
//	nnuetool random -hidden 256 -act screlu -out random256.bin
//	nnuetool eval net.bin shard.bin [-n 20]
//
// random writes a network with small random weights: it plays nonsense,
// and exists to measure what the network costs the search (cmd/profile
// -evalfile) before a trained one exists. eval prints the network's
// score of each record of a training-data shard (White's point of view,
// like the shard's own scores) — the Go side of the Python/Go cross-check.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/nnue"
	"dyanis-chess/internal/trainingdata"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "info":
		err = info(os.Args[2:])
	case "random":
		err = random(os.Args[2:])
	case "eval":
		err = evalShard(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: nnuetool info net.bin | random -hidden H -act crelu|screlu -out net.bin [-seed N] | eval net.bin shard.bin [-n N]")
	os.Exit(2)
}

func info(args []string) error {
	if len(args) != 1 {
		usage()
	}
	n, err := nnue.Load(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s: inputs %d, hidden %d, activation %v, QA %d, QB %d, scale %d, output buckets %d\n",
		args[0], nnue.Inputs, n.Hidden, n.Activation, n.QA, n.QB, n.Scale, n.Buckets)
	return nil
}

func random(args []string) error {
	fs := flag.NewFlagSet("random", flag.ExitOnError)
	hidden := fs.Int("hidden", 256, "hidden layer size")
	act := fs.String("act", "screlu", "activation: crelu or screlu")
	out := fs.String("out", "", "output file")
	seed := fs.Int64("seed", 1, "random seed")
	fs.Parse(args)
	if *out == "" {
		usage()
	}
	a := nnue.SCReLU
	switch strings.ToLower(*act) {
	case "screlu":
	case "crelu":
		a = nnue.CReLU
	default:
		return fmt.Errorf("unknown activation %q", *act)
	}
	n, err := nnue.New(*hidden, a, 255, 64, 400)
	if err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(*seed))
	for _, s := range [][]int16{n.W1, n.B1, n.W2} {
		for i := range s {
			s[i] = int16(rng.Intn(129) - 64)
		}
	}
	data, err := n.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(*out, data, 0o644)
}

func evalShard(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	limit := fs.Int("n", 0, "records to print (0 = all)")
	if len(args) < 2 {
		usage()
	}
	fs.Parse(args[2:])
	n, err := nnue.Load(args[0])
	if err != nil {
		return err
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	if len(data)%trainingdata.RecordSize != 0 {
		return fmt.Errorf("%s: not a whole number of records", args[1])
	}
	count := len(data) / trainingdata.RecordSize
	if *limit > 0 && *limit < count {
		count = *limit
	}
	acc := n.NewAccumulator()
	for i := 0; i < count; i++ {
		b, _, err := trainingdata.Decode(data[i*trainingdata.RecordSize:])
		if err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
		n.Refresh(acc, b)
		s := n.Evaluate(acc, b.SideToMove, b.Occupied().Count())
		if b.SideToMove == board.Black {
			s = -s
		}
		fmt.Printf("%s | %d\n", b.ToFEN(), s)
	}
	return nil
}
