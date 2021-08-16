package strategy

import (
	"fmt"
	"time"

	"github.com/rohitsakala/strategies/pkg/broker"
)

type TwelveThirtyStrategy struct {
	StartTime time.Time
	EndTime   time.Time
	Broker    broker.Broker
	TimeZone  time.Location
}

func NewTwelveThirtyStrategy(broker broker.Broker, timeZone time.Location) TwelveThirtyStrategy {
	return TwelveThirtyStrategy{
		StartTime: time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 12, 25, 0, 0, &timeZone),
		EndTime:   time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 15, 20, 0, 0, &timeZone),
		Broker:    broker,
		TimeZone:  timeZone,
	}
}

func (t TwelveThirtyStrategy) Start() {
	currentTime := time.Now()
	if currentTime.After(t.StartTime) && currentTime.Before(t.EndTime) {
		// Check if positions are already present

	} else {
		// Check if positions are present
		t.positionsPresent()
	}
}

func (t TwelveThirtyStrategy) Stop() error {
	return nil
}

func (t TwelveThirtyStrategy) getATMStrike() (float64, error) {
	strikePrice, err := t.Broker.GetLTP("NIFTY 50")
	if err != nil {
		return 0, err
	}

	return strikePrice, nil
}

func (t TwelveThirtyStrategy) positionsPresent() (bool, error) {
	strikePrice, err := t.getATMStrike()
	if err != nil {
		return false, err
	}

	var atmStrikePrice int

	moduleValue := strikePrice - 50
	if moduleValue > 25 {
		difference := 50 - moduleValue
		atmStrikePrice = int(strikePrice + difference)
	} else {
		atmStrikePrice = int(strikePrice - moduleValue)
	}

	// Weekly or Monthly ?
	fmt.Println(atmStrikePrice)

	return true, nil
}
