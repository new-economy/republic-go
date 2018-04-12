package hyper

import (
	"context"
	"log"
	"sync"
	"time"
)

type HeightContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

var HeightContexts = map[uint64]HeightContext{}

type Buffer struct {
	mu       *sync.RWMutex
	chanSets map[uint64]ChannelSet
}

func NewBuffer() Buffer {
	return Buffer{
		mu:       &sync.RWMutex{},
		chanSets: map[uint64]ChannelSet{},
	}
}

func ProcessBuffer(chanSetIn ChannelSet, validator Validator) ChannelSet {
	buffer, doneCh := ProduceBuffer(chanSetIn, validator)
	return ConsumeBuffer(buffer, doneCh, validator)
}

func ProduceBuffer(chanSetIn ChannelSet, validator Validator) (Buffer, chan struct{}) {
	doneCh := make(chan struct{})
	sb := validator.SharedBlocks()
	buffer := NewBuffer()
	go func() {
		defer close(doneCh)
		defer time.Sleep(10 * time.Second)
		defer log.Println("Closing done channel")
		for {
			h := sb.ReadHeight()
			select {
			case proposal, ok := <-chanSetIn.Proposal:
				if !ok {
					log.Println("returning because of closed proposal channel")
					return
				}
				if proposal.Height < h {
					continue
				}
				buffer.mu.Lock()
				if _, ok := buffer.chanSets[proposal.Height]; !ok {
					buffer.chanSets[proposal.Height] = EmptyChannelSet(validator.Threshold())
				}
				buffer.chanSets[proposal.Height].Proposal <- proposal
				buffer.mu.Unlock()

			case prepare, ok := <-chanSetIn.Prepare:
				if !ok {
					return
				}
				if prepare.Height < h {
					continue
				}
				buffer.mu.Lock()
				if _, ok := buffer.chanSets[prepare.Height]; !ok {
					buffer.chanSets[prepare.Height] = EmptyChannelSet(validator.Threshold())
				}
				buffer.chanSets[prepare.Height].Prepare <- prepare
				buffer.mu.Unlock()

			case commit, ok := <-chanSetIn.Commit:
				if !ok {
					return
				}
				if commit.Height < h {
					continue
				}
				buffer.mu.Lock()
				if _, ok := buffer.chanSets[commit.Height]; !ok {
					buffer.chanSets[commit.Height] = EmptyChannelSet(validator.Threshold())
				}
				buffer.chanSets[commit.Height].Commit <- commit
				buffer.mu.Unlock()

			case fault, ok := <-chanSetIn.Fault:
				if !ok {
					return
				}
				if fault.Height < h {
					continue
				}
				buffer.mu.Lock()
				if _, ok := buffer.chanSets[fault.Height]; !ok {
					buffer.chanSets[fault.Height] = EmptyChannelSet(validator.Threshold())
				}
				buffer.chanSets[fault.Height].Fault <- fault
				buffer.mu.Unlock()
			}
		}
	}()
	return buffer, doneCh
}

func ConsumeBuffer(buffer Buffer, doneCh chan struct{}, validator Validator) ChannelSet {

	sb := validator.SharedBlocks()
	chanSetOut := EmptyChannelSet(validator.Threshold())
	height := sb.ReadHeight()

	ctx, cancel := context.WithCancel(context.Background())
	ictx := HeightContext{
		ctx:    ctx,
		cancel: cancel,
	}
	HeightContexts[height] = ictx

	buffer.mu.RLock()
	log.Println("Start copying", height)
	go chanSetOut.Copy(ctx, buffer.chanSets[0])
	buffer.mu.RUnlock()

	go func() {
		for {
			select {
			case <-doneCh:
				if _, ok := HeightContexts[height]; ok {
					HeightContexts[height].cancel()
				}
				return
			default:
				if height == sb.ReadHeight() {
					continue
				} else {
					newHeight := sb.ReadHeight()
					ctx, cancel := context.WithCancel(context.Background())

					hctx := HeightContext{
						ctx:    ctx,
						cancel: cancel,
					}

					HeightContexts[newHeight] = hctx
					if _, ok := HeightContexts[height]; ok {
						log.Println("Stop copying", height)
						HeightContexts[height].cancel()
					}

					buffer.mu.RLock()
					log.Println("Start copying", newHeight)
					go chanSetOut.Copy(ctx, buffer.chanSets[newHeight])
					buffer.mu.RUnlock()

					for i := height; i < newHeight; i++ {
						buffer.mu.Lock()
						delete(buffer.chanSets, i)
						buffer.mu.Unlock()
					}
					height = newHeight
				}
			}
		}
	}()

	return chanSetOut
}
