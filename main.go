package core

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"
)

// Run starts an echoing Tox client for now.
func (context *Context) Run() error {
	if context == nil {
		return errors.New("Context is nil!")
	}
	// print address
	fmt.Printf("Name: %s\nID: %s\n", context.Name, context.Address)

	isRunning := true

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	tox := context.tox

	for isRunning {
		temp, _ := tox.IterationInterval()
		intervall := time.Duration(temp) * time.Millisecond
		select {
		case <-c:
			fmt.Println("Killing")
			isRunning = false
		case <-time.Tick(intervall):
			err := tox.Iterate()
			if err != nil {
				return err
			}
		}
	}
	// make sure to resafe incase something happened (new friends)
	context.Store()
	return nil
}

/*
Kill and stop Tinzenite.
*/
func (context *Context) Kill() {
	context.tox.Kill()
}
