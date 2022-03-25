package statistic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	logw "github.com/andriiyaremenko/logwriter"
	"github.com/andriiyaremenko/logwriter/color"
	"github.com/andriiyaremenko/mg/dto"
	"github.com/pkg/errors"
)

type Agent struct {
	sc  *Collector
	cl  *http.Client
	log *log.Logger

	statsCh chan []dto.TargetStatistic
	done    chan struct{}
}

func (a *Agent) AddStatistic(stats ...dto.TargetStatistic) {
	a.statsCh <- stats
}

func (a *Agent) Done() <-chan struct{} {
	return a.done
}

func (a *Agent) start(ctx context.Context, statEndpoint string) {
	go func() {
		ticker := time.NewTicker(time.Minute)

	loop:
		for {
			// select chooses the case at random if several cases are available.
			// We want to force it to check ctx.Done first:
			select {
			case <-ctx.Done():
				ticker.Stop()
				a.sendStatistics(statEndpoint)
				a.log.Println("shutting down statistic collection agent")
				close(a.done)

				return
			default:
			}

			// Then check if it's time te send statistic:
			select {
			case <-ticker.C:
				a.sendStatistics(statEndpoint)
				continue loop
			default:
			}

			// And then do whichever comes first:
			select {
			case <-ctx.Done():
				ticker.Stop()
				a.sendStatistics(statEndpoint)
				a.log.Println("shutting down statistic collection agent")
				close(a.done)

				return
			case stat := <-a.statsCh:
				a.sc.Append(stat...)
			case <-ticker.C:
				a.sendStatistics(statEndpoint)
			}
		}
	}()
}

func (a *Agent) sendStatistics(statEndpoint string) {
	for target, stat := range a.sc.Unload() {
		statEndpoint := fmt.Sprintf(statEndpoint, target)

		a.log.Println(
			logw.Info.WithMessage(
				"sending statistic to %s",
				color.ColorizeText(color.ANSIColorBlue, statEndpoint),
			),
		)

		b, err := json.Marshal(stat)
		if err != nil {
			a.log.Println(logw.Error, err)
			continue
		}

		r, err := a.cl.Post(statEndpoint, "application/json", bytes.NewReader(b))
		if err != nil {
			a.log.Println(logw.Error, err)
			continue
		}

		if err := r.Body.Close(); err != nil {
			a.log.Println(logw.Error, err)
		}

		if r.StatusCode >= 300 {
			a.log.Println(logw.Error, errors.Errorf("failed to send statistics: http response code %s", r.Status))
			continue
		}

		a.log.Printf("statistic was sent, response status code %s", r.Status)
	}
}
