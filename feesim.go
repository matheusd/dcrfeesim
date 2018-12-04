// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/fees"
	"github.com/decred/slog"
)

type testCase struct {
	simCfg          simulatorConfig
	estCfg          fees.EstimatorConfig
	testTargetConfs []int32
}

type estimatorUnexportedFuncs interface {
	ProcessMinedTransaction(blockHeight int64, txh *chainhash.Hash)
	UpdateMovingAverages(newHeight int64)
	UpdateDatabase() error
}

var (
	sim     *simulator
	feesLog = slog.Disabled

	testCases = []testCase{
		// Test Case 01: test scenario where blocks still aren't that filled and
		// all transactions are published with a minimum fee rate of 0.0001
		// dcr/KB
		{
			simCfg: simulatorConfig{
				nbTxsCoef:      250.0,
				txSizeCoef:     1000.0,
				minimumFeeRate: 1e4,
				feeRateCoef:    2.5e4,
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 1e4,
				MaxBucketFee: 4e5,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 3, 4, 5, 6, 8, 16, 32},
		},

		// TestCase 02 test scenario where mempool is filled 99% of the time
		{
			simCfg: simulatorConfig{
				nbTxsCoef:      320.0,
				txSizeCoef:     1000.0,
				minimumFeeRate: 1e4,
				feeRateCoef:    2.5e4,
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 1e4,
				MaxBucketFee: 4e5,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 4, 6, 8, 12, 18, 24, 32},
		},

		// TestCase 03 test scenario where there are no minimum relay fees
		{
			simCfg: simulatorConfig{
				nbTxsCoef:      250.0,
				txSizeCoef:     1000.0,
				minimumFeeRate: 0,
				feeRateCoef:    2.5e4,
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 100,
				MaxBucketFee: 4e5,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 3, 4, 5, 6, 8, 16, 32},
		},

		// TestCase 04 test scenario where there are no minimum relay fees and
		// transactions are generated at a higher rate and using a higher
		// confirmation window
		{
			simCfg: simulatorConfig{
				nbTxsCoef:      320.0,
				txSizeCoef:     1000.0,
				minimumFeeRate: 0,
				feeRateCoef:    2.5e4,
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 100,
				MaxBucketFee: 4e5,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 4, 6, 8, 12, 16, 32},
		},

		// TestCase 05: Same as test 01, with lower contention rate
		{
			simCfg: simulatorConfig{
				nbTxsCoef:      105.0,
				txSizeCoef:     1000.0,
				minimumFeeRate: 1e4,
				feeRateCoef:    2.5e4,
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 1e4,
				MaxBucketFee: 4e5,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 3, 4, 5, 6, 8, 10, 16},
		},

		// TestCase 06: Same as test 01, with lower contention rate and lower
		// fee spread distribution. Max fee bucket and FeeRateStep are adjusted
		// to improve estimates.
		{
			simCfg: simulatorConfig{
				nbTxsCoef:               105.0,
				txSizeCoef:              1000.0,
				minimumFeeRate:          1e4,
				feeRateCoef:             1e2,
				feeRateHistReportValues: []uint32{9999, 10000, 10001, 10070, 10250, 10500, 11000, 15000},
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 1e4,
				MaxBucketFee: 25000,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 3, 4, 5, 6, 8, 10, 16},
		},

		// TestCase 07: Same as test 01 but with slighly higher contention rate
		{
			simCfg: simulatorConfig{
				nbTxsCoef:      125.0,
				txSizeCoef:     1000.0,
				minimumFeeRate: 1e4,
				feeRateCoef:    2.5e4,
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 1e4,
				MaxBucketFee: 4e5,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 4, 6, 8, 16, 24, 32},
		},

		// testcase 08: similar to what was found to be the current load as of
		// 2018-08
		{
			simCfg: simulatorConfig{
				nbTxsCoef:      20.0,
				txSizeCoef:     500.0,
				minimumFeeRate: 100000,
				feeRateCoef:    1e3,
			},
			estCfg: fees.EstimatorConfig{
				MaxConfirms:  32,
				MinBucketFee: 1e4,
				MaxBucketFee: 4e5,
				FeeRateStep:  1.1,
			},
			testTargetConfs: []int32{1, 2, 3, 4, 5, 6, 8, 16, 32},
		},
	}
)

func main() {

	useFeesFile := flag.Bool("usefeesfile", false, "Use fees file")
	lenSimulation := flag.Int("lensimulation", 288*30, "Number of blocks to run the simulation for")
	flag.Parse()

	if len(flag.Args()) < 1 {
		fmt.Println("Please specify the test number")
		os.Exit(1)
	}

	testNb, err := strconv.Atoi(flag.Arg(0))
	if err != nil {
		fmt.Println("Please specify a number as test case")
		os.Exit(1)
	}
	if testNb-1 > len(testCases) {
		fmt.Printf("Please specify a test in the range of 1-%d\n", len(testCases))
		os.Exit(1)
	}
	actualTest := testCases[testNb-1]

	if *useFeesFile {
		actualTest.estCfg.DatabaseFile = "fees.db"
	}

	sim := newSimulator(&actualTest.simCfg)
	var newTxs, minedTxs []*simTx
	memPool := make(txPool, 0)
	heap.Init(&memPool)

	estimator, err := fees.NewEstimator(&actualTest.estCfg)
	if err != nil {
		panic(err)
	}

	start := time.Now()

	estimatorFuncs := fees.ExposeInternalFunctions(estimator).(estimatorUnexportedFuncs)

	// simulate a bunch of blocks. At every iteration, this simulates:
	// - a miner generating a new block from the current memPool
	// - some new transactions appearing in the network and being added to the
	// outstanding mempool
	startHeight := uint32(1)
	endHeight := startHeight + uint32(*lenSimulation) - 1
	reportedPerc := uint32(0)
	for h := startHeight; h < endHeight; h++ {
		minedTxs = sim.mineTransactions(h, &memPool)
		newTxs = sim.genTransactions(h, &memPool)
		sim.trackHistograms(minedTxs, newTxs, h)

		// Update the estimator (this is thing that would actually run in the
		// mempool of a full node once a new block has been fonud)
		estimatorFuncs.UpdateMovingAverages(int64(h))
		for _, tx := range minedTxs {
			estimatorFuncs.ProcessMinedTransaction(int64(h), &tx.txHash)
		}

		// This would happen as new transactions are entering the memPool
		for _, tx := range newTxs {
			estimator.AddMemPoolTransaction(&tx.txHash, int64(tx.fee), int64(tx.size))
		}

		if *useFeesFile {
			estimatorFuncs.UpdateDatabase()
		}

		perc := (h - startHeight) * 100 / uint32(*lenSimulation)
		if perc-reportedPerc >= 10 {
			fmt.Fprintf(os.Stderr, "%d%% ", perc)
			reportedPerc = perc
		}
	}
	fmt.Fprintf(os.Stderr, "\n\n")

	estimator.Close()

	end := time.Now()
	diff := time.Duration(end.UnixNano() - start.UnixNano())
	fmt.Fprintf(os.Stderr, "Total time: %s\n", diff.String())

	// Simulation has ended (eg: full node has synced)
	// Let's now try to estimate the fees.

	fmt.Println("=== Test Case Setup ===")
	fmt.Printf("%+v\n\n", actualTest)

	// Let's try generating fee rate estimates for a number of different target
	// ranges at the same success pct (this is roughly what bitcoin core does)
	fmt.Println("=== Fees to use for target confirmations ===")
	l1 := ""
	l2 := ""
	for _, t := range actualTest.testTargetConfs {
		l1 += fmt.Sprintf("%12d", t)
		fee, err := estimator.EstimateFee(t)
		if err != nil {
			if err == fees.ErrNoSuccessPctBucketFound {
				l2 += "   noSuccBkt"
			} else if err == fees.ErrNotEnoughTxsForEstimate {
				l2 += "   notEnghTx"
			} else if _, is := err.(fees.ErrTargetConfTooLarge); is {
				l2 += " cftTooLarge"
			} else {
				l2 += "         err"
			}
		} else {
			l2 += fmt.Sprintf("%12.8f", float64(fee)/1e8)
		}
	}
	fmt.Printf("%s\n%s\n\n", l1, l2)

	// report the histogram of the simulated transactions to see if they are
	// reasonable
	fmt.Println("=== Histograms for simulated data ===")
	sim.reportSimHistograms()

	// Let's see the internal state of the estimator
	fmt.Println("=== Internal Estimator State ===")
	fmt.Println(estimator.DumpBuckets())
}
