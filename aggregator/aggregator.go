package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
        "runtime"
        "path"

	"github.com/kardianos/osext"
	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	agg "github.com/nsip/nias-go-naplan-registration/aggregator/lib"
	"github.com/nats-io/nats"
	"github.com/nats-io/nuid"
	"github.com/wildducktheories/go-csv"
)

func main() {

        _, currentFilePath, _, _ := runtime.Caller(0)

	// set up nats broker connections
	nc, con_error := nats.Connect(nats.DefaultURL)
	if con_error != nil {
		log.Fatalf("\n\nCannot create connection to NATS server\n...service aborting\n\n")
	}

	ec, enc_error := nats.NewEncodedConn(nc, nats.JSON_ENCODER)
	if enc_error != nil {
		log.Fatalf("\n\nCannot create json-encoded connection to NATS server\n...service aborting\n\n")
	}
	log.Println("NATS connections established...")

	var mutex = &sync.Mutex{}

	// structure to aggregate error records for a transaction
	dm := make(map[string][]*agg.ValidationError)
	_, sub_err := ec.Subscribe("validation.errors", func(ve *agg.ValidationError) {

		mutex.Lock()
		ls, ok := dm[ve.TxID]
		mutex.Unlock()
		if !ok {
			ls = make([]*agg.ValidationError, 0)
			mutex.Lock()
			dm[ve.TxID] = ls
			mutex.Unlock()
		}

		mutex.Lock()
		dm[ve.TxID] = append(ls, ve)
		mutex.Unlock()
	})

	// capture progress information from validation services
	st := make(map[string]map[string]int)
	_, sub_err = ec.Subscribe("validation.status", func(pn *agg.ProcessingNotification) {

		mm, ok := st[pn.TxID]
		if !ok {
			mm = make(map[string]int)
			st[pn.TxID] = mm
		}
		mm[pn.Vtype]++

	})

	// transaciton summary for any given input file
	summary := &agg.TransactionSummary{}
	_, sub_err = ec.Subscribe("validation.tx", func(ts *agg.TransactionSummary) {

		summary = ts

		mm, ok := st[ts.TxID]
		if !ok {
			mm = make(map[string]int)
			st[ts.TxID] = mm
		}
		mm["Total"] = ts.RecordCount

	})

	if sub_err != nil {
		log.Fatalf("\n\nCannot create subscriptions to NATS topics\n...service aborting\n\n")
	}

	e := echo.New()

	// Middleware
	e.Use(mw.Logger())
	e.Use(mw.Recover())

	exeDir, _ := osext.ExecutableFolder()
	log.Println(exeDir)

	// main javascript web client
	e.ServeFile("/validation", path.Join(path.Join(path.Dir(currentFilePath), "public"), "validation.html"))

	// support files
	e.Static("/css/", path.Join(path.Join(path.Dir(currentFilePath), "public"), "css"))
	e.Static("/images/", path.Join(path.Join(path.Dir(currentFilePath), "public"), "images"))
	e.Static("/javascript/", path.Join(path.Join(path.Dir(currentFilePath), "public"), "javascript"))
	e.Static("/fileupload/", path.Join(path.Join(path.Dir(currentFilePath), "public"), "fileupload"))

	// Routes
	// The endpoint to post input csv files to
	e.Post("/naplan/reg/:stateID", func(c *echo.Context) error {

		reader := csv.WithIoReader(c.Request().Body)
		records, err := csv.ReadAll(reader)
		log.Printf("records received: %v", len(records))
		if err != nil {
			return err
		}
		txID := nuid.Next()
		ts := agg.TransactionSummary{txID, len(records)}
		err = ec.Publish("validation.tx", ts)
		if err != nil {
			return err
		}

		for i, r := range records {

			r := r.AsMap()
			r["OriginalLine"] = strconv.Itoa(i + 1)
			r["TxID"] = txID
			// log.Printf("%+v\n\n", r)

			err := ec.Publish("validation.naplan", r)
			if err != nil {
				return err
			}
		}
		log.Println("...all records converted & published for validation")

		return c.String(http.StatusOK, txID)
	})

	// SSE endpoint that provides status/progress updates
	e.Get("/statusfeed/:txID", func(c *echo.Context) error {

		txID := c.Param("txID")

		c.Response().Header().Set(echo.ContentType, "text/event-stream")
		c.Response().WriteHeader(http.StatusOK)

		mutex.Lock()
		m := st[txID]
		mutex.Unlock()

		type result struct {
			Vtype string `json:"v_type"`
			Count int    `json:"count"`
		}
		results := make([]result, 0)

		for key, value := range m {
			r := result{key, value}
			results = append(results, r)
		}
		sm, _ := json.Marshal(results)

		suffix := string(sm) + "\n\n"
		if _, err := c.Response().Write([]byte("data: " + suffix)); err != nil {
			log.Println(err)
		}

		c.Response().Flush()

		return nil

	})

	// SSE endpoint to announce when all messages in a transaction have been processed
	e.Get("/readyfeed/:txID", func(c *echo.Context) error {

		txID := c.Param("txID")

		c.Response().Header().Set(echo.ContentType, "text/event-stream")
		c.Response().WriteHeader(http.StatusOK)

		mutex.Lock()
		total := st[txID]["Total"]
		mutex.Unlock()
		log.Println("available records: ", total)

		mutex.Lock()
		summ := st[txID]
		mutex.Unlock()
		for _, value := range summ {
			if total != value {
				return nil
			}
		}

		suffix := string(txID) + "\n\n"
		if _, err := c.Response().Write([]byte("data: " + suffix)); err != nil {
			log.Println(err)
		}

		c.Response().Flush()

		return nil

	})

	// get the errors data for a given transaction
	e.Get("/data/:txID", func(c *echo.Context) error {

		txID := c.Param("txID")

		mutex.Lock()
		data := dm[txID]
		mutex.Unlock()

		err := c.JSON(http.StatusOK, data)

		// data is only served once, so delete once provided
		mutex.Lock()
		delete(dm, txID)
		mutex.Unlock()

		return err

	})

	// Start server
	log.Println("Starting aggregation-ui server...")
	log.Println("Service is listening on localhost:1324")

	e.Run(":1324")
}