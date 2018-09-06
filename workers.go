package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
)

type cmdGenerator func(string) *exec.Cmd

type Mapper struct {
	workers         int
	echoInput       bool
	outputDelimiter string

	generateCmd cmdGenerator
}

func (m *Mapper) Map(inputChannel chan string) (outputChannel chan string) {
	wg := &sync.WaitGroup{}
	outputChannel = make(chan string, 1<<4)

	// Kick off all the worker routines
	for w := 0; w < m.workers; w++ {
		wg.Add(1)
		go m.executeTask(wg, inputChannel, outputChannel)
	}

	go func() {
		// Wait until all workers complete
		wg.Wait()

		// Close the output channel, indicating all workers have completed, and all input was processed
		close(outputChannel)
	}()

	return
}

func (m *Mapper) executeTask(wg *sync.WaitGroup, inputChannel, outputChannel chan string) {
	for line := range inputChannel {
		process := m.generateCmd(line)
		process.Stderr = os.Stderr
		stdout, err := process.StdoutPipe()
		if err != nil {
			log.Fatal(err)
		}
		if err := process.Start(); err != nil {
			log.Fatal(err)
		}
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			out := scanner.Text()
			if m.echoInput {
				out = fmt.Sprintf("%s%s%s", line, m.outputDelimiter, out)
			}
			outputChannel <- out
		}
		if err := process.Wait(); err != nil {
			log.Fatal(err)
		}
	}
	wg.Done()
}

