package twelvethirty

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/avast/retry-go"
	"github.com/rohitsakala/strategies/pkg/broker"
	"github.com/rohitsakala/strategies/pkg/database"
	"github.com/rohitsakala/strategies/pkg/models"
	"github.com/rohitsakala/strategies/pkg/utils/duration"
	"github.com/rohitsakala/strategies/pkg/utils/options"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	TwelveThirtyStrategyDatabaseName = "twelvethirty"
)

type TwelveThirtyStrategy struct {
	StartTime time.Time
	EndTime   time.Time
	Broker    broker.Broker
	TimeZone  time.Location
	Database  database.Database
	Filter    bson.M
}

func NewTwelveThirtyStrategy(broker broker.Broker, timeZone time.Location, database database.Database) (TwelveThirtyStrategy, error) {
	err := database.CreateCollection(TwelveThirtyStrategyDatabaseName)
	if err != nil {
		return TwelveThirtyStrategy{}, err
	}

	return TwelveThirtyStrategy{
		StartTime: time.Date(time.Now().In(&timeZone).Year(), time.Now().In(&timeZone).Month(), time.Now().In(&timeZone).Day(), 12, 25, 0, 0, &timeZone),
		EndTime:   time.Date(time.Now().In(&timeZone).Year(), time.Now().In(&timeZone).Month(), time.Now().In(&timeZone).Day(), 15, 30, 0, 0, &timeZone),
		Broker:    broker,
		TimeZone:  timeZone,
		Database:  database,
	}, nil
}

func (t *TwelveThirtyStrategy) fetchData() (TwelveThiryStrategyPositions, error) {
	var data TwelveThiryStrategyPositions

	collectionRaw, err := t.Database.GetCollection(bson.D{}, TwelveThirtyStrategyDatabaseName)
	if err != nil {
		return TwelveThiryStrategyPositions{}, err
	}
	if len(collectionRaw) <= 0 {
		insertID, err := t.Database.InsertCollection(data, TwelveThirtyStrategyDatabaseName)
		if err != nil {
			return data, err
		}
		t.Filter = bson.M{
			"_id": insertID,
		}
		return data, nil
	}

	dataBytes, err := bson.Marshal(collectionRaw)
	if err != nil {
		return TwelveThiryStrategyPositions{}, err
	}
	err = bson.Unmarshal(dataBytes, &data)
	if err != nil {
		return TwelveThiryStrategyPositions{}, err
	}

	t.Filter = bson.M{
		"_id": collectionRaw["_id"],
	}

	return data, nil
}

func (t *TwelveThirtyStrategy) Start() error {
	var data TwelveThiryStrategyPositions

	data, err := t.fetchData()
	if err != nil {
		return err
	}

	log.Printf("Waiting for 12:25 pm to 12:35 pm....")

	startTime := time.Date(time.Now().In(&t.TimeZone).Year(), time.Now().In(&t.TimeZone).Month(), time.Now().In(&t.TimeZone).Day(), 12, 25, 0, 0, &t.TimeZone)
	endTime := time.Date(time.Now().In(&t.TimeZone).Year(), time.Now().In(&t.TimeZone).Month(), time.Now().In(&t.TimeZone).Day(), 12, 35, 0, 0, &t.TimeZone)

	for {
		if !duration.ValidateTime(startTime, endTime, t.TimeZone) {
			time.Sleep(1 * time.Minute)
		} else {
			log.Printf("Time : %v", time.Now().In(&t.TimeZone))
			break
		}
	}
	log.Printf("Entering 12:25 pm to 12:35 pm.")

	strikePrice, err := options.GetATM("NIFTY 50", t.Broker)
	if err != nil {
		return err
	}

	ceLeg := data.SellCEOptionPosition
	if data.SellCEOptionPosition.TradingSymbol == "" {
		ceLeg, err = t.calculateLeg("CE", strikePrice)
		if err != nil {
			return err
		}
		log.Printf("Calculating CE Leg.... %s %d", ceLeg.TradingSymbol, ceLeg.Quantity)
		err = t.placeLeg(&ceLeg, "Retrying placing leg")
		if err != nil {
			return err
		}
		log.Printf("Placing CE Leg with Avg Price %f", ceLeg.AveragePrice)
		data.SellCEOptionPosition = ceLeg
	}

	peLeg := data.SellPEOptionPoistion
	if data.SellPEOptionPoistion.TradingSymbol == "" {
		peLeg, err = t.calculateLeg("PE", strikePrice)
		if err != nil {
			return err
		}
		log.Printf("Calculating PE Leg.... %s %d", peLeg.TradingSymbol, peLeg.Quantity)

		err = t.placeLeg(&peLeg, "Retrying placing leg")
		if err != nil {
			return err
		}
		log.Printf("Placing PE Leg with Avg Price %f", peLeg.AveragePrice)
		data.SellPEOptionPoistion = peLeg
	}

	err = t.Database.UpdateCollection(t.Filter, data, "twelvethirty")
	if err != nil {
		return err
	}

	ceStopLossLeg := data.SellCEStopLossOptionPosition
	if data.SellCEStopLossOptionPosition.TradingSymbol == "" {
		ceStopLossLeg, err = t.calculateStopLossLeg(ceLeg)
		if err != nil {
			return err
		}

		err = t.placeLeg(&ceStopLossLeg, "Retrying placing stoploss leg")
		if err != nil {
			return err
		}
		log.Printf("Placing CE StopLoss Leg with Trigger Price %f", ceStopLossLeg.TriggerPrice)
		data.SellCEStopLossOptionPosition = ceStopLossLeg
	}
	peStopLossLeg := data.SellPEStopLossOptionPosition
	if data.SellPEStopLossOptionPosition.TradingSymbol == "" {
		peStopLossLeg, err = t.calculateStopLossLeg(peLeg)
		if err != nil {
			return err
		}

		err = t.placeLeg(&peStopLossLeg, "Retrying placing stoploss leg")
		if err != nil {
			return err
		}
		log.Printf("Placing PE StopLoss Leg with Trigger Price %f", peStopLossLeg.TriggerPrice)
		data.SellPEStopLossOptionPosition = peStopLossLeg
	}

	err = t.Database.UpdateCollection(t.Filter, data, "twelvethirty")
	if err != nil {
		return err
	}

	startTime = time.Date(time.Now().In(&t.TimeZone).Year(), time.Now().In(&t.TimeZone).Month(), time.Now().In(&t.TimeZone).Day(), 15, 25, 0, 0, &t.TimeZone)
	endTime = time.Date(time.Now().In(&t.TimeZone).Year(), time.Now().In(&t.TimeZone).Month(), time.Now().In(&t.TimeZone).Day(), 15, 30, 0, 0, &t.TimeZone)

	log.Printf("Waiting for 3:25 to 3:30 pm....")
	for {
		if !duration.ValidateTime(startTime, endTime, t.TimeZone) {
			time.Sleep(1 * time.Minute)
		} else {
			log.Printf("Time : %v", time.Now().In(&t.TimeZone))
			break
		}
	}

	log.Printf("Cancelling all pending orders...")
	err = t.cancelOrders(&ceStopLossLeg, &peStopLossLeg)
	if err != nil {
		return err
	}
	log.Printf("Cancelled all pending orders.")

	log.Printf("Exiting all current positions...")
	positionList := models.Positions{}
	if ceStopLossLeg.Status != kiteconnect.OrderStatusComplete {
		positionList = append(positionList, ceLeg)
	}
	if peStopLossLeg.Status != kiteconnect.OrderStatusComplete {
		positionList = append(positionList, peLeg)
	}
	err = t.cancelPositions(positionList)
	if err != nil {
		return err
	}
	log.Printf("Exited all current positions.")

	data.SellPEOptionPoistion = models.Position{}
	data.SellCEOptionPosition = models.Position{}
	data.SellPEStopLossOptionPosition = models.Position{}
	data.SellCEStopLossOptionPosition = models.Position{}
	err = t.Database.UpdateCollection(t.Filter, data, "twelvethirty")
	if err != nil {
		return err
	}

	return nil
}

func (t *TwelveThirtyStrategy) cancelOrders(ceStopLoss *models.Position, peStopLoss *models.Position) error {
	err := retry.Do(
		func() error {
			err := t.Broker.CancelOrder(ceStopLoss)
			if err != nil {
				return err
			}
			return nil
		},
		retry.OnRetry(func(n uint, err error) {
			log.Println(fmt.Sprintf("%s %s because %s", "Retrying cancelling order ", ceStopLoss.TradingSymbol, err))
		}),
		retry.Delay(5*time.Second),
		retry.Attempts(5),
	)
	if err != nil {
		return err
	}

	err = retry.Do(
		func() error {
			err := t.Broker.CancelOrder(peStopLoss)
			if err != nil {
				return err
			}
			return nil
		},
		retry.OnRetry(func(n uint, err error) {
			log.Println(fmt.Sprintf("%s %s because %s", "Retrying cancelling order ", peStopLoss.TradingSymbol, err))
		}),
		retry.Delay(5*time.Second),
		retry.Attempts(5),
	)
	if err != nil {
		return err
	}

	return nil
}

func (t *TwelveThirtyStrategy) cancelPositions(positions models.Positions) error {
	for _, position := range positions {
		position.TransactionType = kiteconnect.TransactionTypeBuy
		position.Status = ""
		position.OrderID = ""
		err := t.placeLeg(&position, fmt.Sprintf("%s %s", "Retrying cancelling position ", position.TradingSymbol))
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *TwelveThirtyStrategy) calculateLeg(optionType string, strikePrice float64) (models.Position, error) {
	leg := models.Position{
		Type:            optionType,
		Exchange:        kiteconnect.ExchangeNFO,
		TransactionType: "SELL",
		Product:         kiteconnect.ProductNRML,
		OrderType:       kiteconnect.OrderTypeMarket,
	}

	legSymbol, err := options.GetSymbol("NIFTY", options.WEEK, 0, strikePrice, optionType, t.Broker)
	if err != nil {
		return models.Position{}, err
	}
	leg.TradingSymbol = legSymbol
	lotSize, err := options.GetLotSize(legSymbol, t.Broker)
	if err != nil {
		return models.Position{}, err
	}
	leg.LotSize = lotSize

	lotQuantity, err := strconv.Atoi(os.Getenv("TWELVE_THIRTY_LOT_QUANTITY"))
	if err != nil {
		return models.Position{}, err
	}
	leg.Quantity = lotQuantity * lotSize

	legExpiry, err := options.GetExpiry("NIFTY", options.WEEK, 0, strikePrice, optionType, t.Broker)
	if err != nil {
		return models.Position{}, err
	}
	leg.Expiry = legExpiry

	return leg, nil
}

func (t *TwelveThirtyStrategy) calculateStopLossLeg(leg models.Position) (models.Position, error) {
	leg.TransactionType = kiteconnect.TransactionTypeBuy
	leg.Product = kiteconnect.ProductNRML
	leg.OrderType = kiteconnect.OrderTypeSL
	leg.OrderID = ""
	leg.Status = ""

	stopLossPercentage := 30

	expiryDate := leg.Expiry
	now := time.Now().In(&t.TimeZone)
	diff := expiryDate.Sub(now)

	if int(diff.Hours()) < 0 {
		stopLossPercentage = 70
	} else if int(diff.Hours()/24) == 0 {
		stopLossPercentage = 40
	}
	stopLossPrice := leg.AveragePrice * float64(stopLossPercentage) / 100
	stopLossPrice = stopLossPrice + leg.AveragePrice
	leg.TriggerPrice = float64(int(stopLossPrice*10)) / 10
	leg.Price = float64(int(leg.TriggerPrice) + 5)

	return leg, nil
}

func (t *TwelveThirtyStrategy) placeLeg(leg *models.Position, retryMsg string) error {
	err := retry.Do(
		func() error {
			err := t.Broker.PlaceOrder(leg)
			if err != nil {
				return err
			}
			return nil
		},
		retry.OnRetry(func(n uint, err error) {
			log.Println(fmt.Sprintf("%s because %s", retryMsg, err))
		}),
		retry.Delay(5*time.Second),
		retry.Attempts(5),
	)
	if err != nil {
		return err
	}

	return nil
}
