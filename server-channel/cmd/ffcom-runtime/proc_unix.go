//go:build unix

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// childSysProcAttr põe o filho no próprio grupo de processos: um Ctrl+C no
// terminal (go run local) chega só ao lançador, que decide como parar o
// filho, em vez de os dois receberem SIGINT ao mesmo tempo. No container o
// lançador é PID 1 e o único filho direto é o server-channel (colhido por
// Wait), então não há zumbis a colher.
func childSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func terminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// notifySignals liga SIGTERM/SIGINT a cancel (encerrar) e SIGHUP a hup
// (checar atualização agora; sinais repetidos se fundem). Devolve a função
// que desliga os handlers.
func notifySignals(logger *log.Logger, cancel func(), hup chan<- struct{}) (stop func()) {
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)
	h := make(chan os.Signal, 1)
	signal.Notify(h, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-term:
				logger.Printf("recebeu %v; parando server-channel e saindo", s)
				cancel()
			case <-h:
				select {
				case hup <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(term)
		signal.Stop(h)
		close(done)
	}
}
