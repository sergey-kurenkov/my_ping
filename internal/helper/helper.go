package helper

import (
	"fmt"
	"github.com/montanaflynn/stats"
	"os"
	"time"
)

type RoundtripStat struct {
	Size              int
	Min               time.Duration
	Max               time.Duration
	Mean              time.Duration
	Median            time.Duration
	Percentile99      time.Duration
	StandardDeviation time.Duration
}

func GetStat(roundtrips stats.Float64Data) (RoundtripStat, error) {
	minFloat, err := stats.Min(roundtrips)
	if err != nil {
		return RoundtripStat{}, err
	}

	maxFloat, err := stats.Max(roundtrips)
	if err != nil {
		return RoundtripStat{}, err
	}

	const percentil99 = 99

	percentil99Float, err := stats.Percentile(roundtrips, percentil99)
	if err != nil {
		return RoundtripStat{}, err
	}

	medianFloat, err := stats.Median(roundtrips)
	if err != nil {
		return RoundtripStat{}, err
	}

	averageFloat, err := stats.Mean(roundtrips)
	if err != nil {
		return RoundtripStat{}, err
	}

	sdevFloat, err := stats.StandardDeviation(roundtrips)
	if err != nil {
		return RoundtripStat{}, err
	}

	return RoundtripStat{
		Size:              len(roundtrips),
		Min:               time.Duration(minFloat),
		Max:               time.Duration(maxFloat),
		Mean:              time.Duration(averageFloat),
		Median:            time.Duration(medianFloat),
		Percentile99:      time.Duration(percentil99Float),
		StandardDeviation: time.Duration(sdevFloat),
	}, nil
}

func PrintStat(currentRPS float64, roundtripsCh chan time.Duration, singleTestDuration time.Duration) {
	numRoundtrips := len(roundtripsCh)

	roundtrips := make(stats.Float64Data, numRoundtrips)
	for i := 0; i < numRoundtrips; i++ {
		roundtrip := <-roundtripsCh
		roundtrips[i] = float64(roundtrip)
	}

	if len(roundtrips) == 0 {
		fmt.Println("no roundtrip data")
		return
	}

	realRPS := float64(len(roundtrips)) / (float64(singleTestDuration) / float64(time.Second))

	stat, err := GetStat(roundtrips)
	if err != nil {
		fmt.Printf("myPingTCPServer.printStat: Percentile: %v", err)
		os.Exit(1)
	}

	fmt.Printf("rps: %v, real rps: %.1f, samples: %v, "+
		"min: %v, max: %v, mean: %v, std deviation: %v, median: %v, percentile99: %v\n",
		currentRPS, realRPS, stat.Size,
		stat.Min.Microseconds(), stat.Max.Microseconds(), stat.Mean.Microseconds(),
		stat.StandardDeviation.Microseconds(), stat.Median.Microseconds(),
		stat.Percentile99.Microseconds(),
	)
}

const DefaultMessageSize = 100
