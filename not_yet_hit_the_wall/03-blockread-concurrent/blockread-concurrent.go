package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

const (
	newLine        = byte('\n')
	fileBufferSize = 256 * 1024
)

type worker struct {
	wg         *sync.WaitGroup
	chanIn     chan *[]byte
	chanReturn chan *[]byte
	chanCount  chan int
}

func (w *worker) run() {
	defer w.wg.Done()

	for bytes := range w.chanIn {
		// chanIn 里面的byte，统计有几个换行符，计算出总的行数
		count := 0
		for i := 0; i < len(*bytes); i++ {
			if (*bytes)[i] == newLine {
				count++
			}
		}
		// 一个block 块里面的行数送到 chanCount
		w.chanCount <- count
		w.chanReturn <- bytes
		//fmt.Println(&bytes)
	}
}

type aggregator struct {
	wg        *sync.WaitGroup
	chanCount chan int
	result    *int
}

func (a *aggregator) run() {
	defer a.wg.Done()
	//aggregator 计算 chanCount 里面的数字 求和，得到总的行数
	for j := range a.chanCount {
		*(a.result) += j
	}
}

func main() {
	start := time.Now()
	numWorkerMAX := runtime.GOMAXPROCS(0)
	numWorker := numWorkerMAX - 1
	if numWorkerMAX < 8 {
		numWorker = numWorkerMAX
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer file.Close()

	totalCount := 0

	chanReturn := make(chan *[]byte, 2*numWorkerMAX)
	chanIn := make(chan *[]byte, 2*numWorkerMAX)
	chanCount := make(chan int, 2*numWorkerMAX)

	for i := 0; i < 2*numWorkerMAX; i++ {
		tempBuff := make([]byte, fileBufferSize)
		chanReturn <- &tempBuff
	}

	wg := &sync.WaitGroup{}
	wgAggregate := &sync.WaitGroup{}

	wgAggregate.Add(1)
	ag := &aggregator{
		wg:        wgAggregate,
		chanCount: chanCount,
		result:    &totalCount,
	}
	go ag.run()

	wg.Add(numWorker)
	for i := 0; i < numWorker; i++ {
		w := &worker{
			wg:         wg,
			chanIn:     chanIn,
			chanReturn: chanReturn,
			chanCount:  chanCount,
		}

		go w.run()
	}

	for {
		bf := <-chanReturn
		n, err := file.Read(*bf)
		if err == io.EOF {
			break
		}

		if n != fileBufferSize {
			*bf = (*bf)[:n]
		}

		chanIn <- bf

	}

	close(chanIn)
	wg.Wait()

	close(chanCount)
	wgAggregate.Wait()

	close(chanReturn)

	fmt.Println("Number of line:", totalCount)
	fmt.Printf("Finished in %v\n", time.Since(start))
}
