package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

func lineCount(b []byte) (count int) {
	for i := 0; i < len(b); i++ {
		// 每到一个换行符，行数加1
		if b[i] == '\n' {
			count++
		}
	}
	return count
}

func main() {
	start := time.Now()

	buf := make([]byte, 256*1024)
	file, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}

	defer file.Close()

	count := 0
	for {
		// 每次读取256k byte
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		c := lineCount(buf[:n])
		count += c
	}

	fmt.Println("Total lines:", count)
	fmt.Printf("Total time: %v\n", time.Since(start))
}
