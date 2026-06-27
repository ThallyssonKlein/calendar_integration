package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	godotenv.Load()
}

type EventTime struct {
	DateTime string `bson:"dateTime,omitempty" json:"dateTime,omitempty"`
	TimeZone string `bson:"timeZone,omitempty" json:"timeZone,omitempty"`
	Date     string `bson:"date,omitempty" json:"date,omitempty"`
}

type Person struct {
	Email       string `bson:"email,omitempty" json:"email,omitempty"`
	DisplayName string `bson:"displayName,omitempty" json:"displayName,omitempty"`
	Self        bool   `bson:"self,omitempty" json:"self,omitempty"`
}

type Attendee struct {
	Email          string `bson:"email,omitempty" json:"email,omitempty"`
	DisplayName    string `bson:"displayName,omitempty" json:"displayName,omitempty"`
	ResponseStatus string `bson:"responseStatus,omitempty" json:"responseStatus,omitempty"`
	Self           bool   `bson:"self,omitempty" json:"self,omitempty"`
}

type Event struct {
	GoogleID         string    `bson:"id,omitempty" json:"id,omitempty"`
	RecurringEventID string    `bson:"recurringEventId,omitempty" json:"recurringEventId,omitempty"`
	HtmlLink         string    `bson:"htmlLink,omitempty" json:"htmlLink,omitempty"`
	HangoutLink      string    `bson:"hangoutLink,omitempty" json:"hangoutLink,omitempty"`
	Summary          string    `bson:"summary,omitempty" json:"summary,omitempty"`
	Description      string    `bson:"description,omitempty" json:"description,omitempty"`
	Location         string    `bson:"location,omitempty" json:"location,omitempty"`
	Start            EventTime `bson:"start,omitempty" json:"start,omitempty"`
	End              EventTime `bson:"end,omitempty" json:"end,omitempty"`
	Status           string    `bson:"status,omitempty" json:"status,omitempty"`
	Creator          Person    `bson:"creator,omitempty" json:"creator,omitempty"`
	Organizer        Person    `bson:"organizer,omitempty" json:"organizer,omitempty"`
	Attendees        []Attendee `bson:"attendees,omitempty" json:"attendees,omitempty"`
}

var (
	col     *mongo.Collection
	once    sync.Once
	initErr error
)

func getCollection() (*mongo.Collection, error) {
	once.Do(func() {
		uri := os.Getenv("MONGODB_URI")
		dbName := os.Getenv("MONGODB_DB")
		if dbName == "" {
			dbName = "calendar"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil {
			initErr = err
			return
		}
		col = client.Database(dbName).Collection("events")
	})
	return col, initErr
}

var router = func() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("POST /events", batchUpsertEvents)
	m.HandleFunc("GET /docs", getDocs)
	return m
}()

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)
}

func batchUpsertEvents(w http.ResponseWriter, r *http.Request) {
	var events []Event
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: expected array of events")
		return
	}
	if len(events) == 0 {
		writeError(w, http.StatusBadRequest, "events array must not be empty")
		return
	}

	c, err := getCollection()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	models := make([]mongo.WriteModel, 0, len(events))
	for _, e := range events {
		if e.GoogleID == "" {
			writeError(w, http.StatusBadRequest, "each event must have an 'id' field")
			return
		}
		m := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"id": e.GoogleID}).
			SetUpdate(bson.M{"$set": e}).
			SetUpsert(true)
		models = append(models, m)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := c.BulkWrite(ctx, models)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"upsertedCount": result.UpsertedCount,
		"modifiedCount": result.ModifiedCount,
		"matchedCount":  result.MatchedCount,
	})
}

var schema = map[string]interface{}{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"title":   "Batch Upsert Events",
	"description": "POST /events — upserts a batch of Google Calendar events. Each event is matched by 'id' (Google Calendar event ID).",
	"type": "array",
	"items": map[string]interface{}{
		"type":     "object",
		"required": []string{"id"},
		"properties": map[string]interface{}{
			"id":               map[string]string{"type": "string", "description": "Google Calendar event ID (used as upsert key)"},
			"recurringEventId": map[string]string{"type": "string"},
			"htmlLink":         map[string]string{"type": "string", "format": "uri"},
			"hangoutLink":      map[string]string{"type": "string", "format": "uri"},
			"summary":          map[string]string{"type": "string"},
			"description":      map[string]string{"type": "string"},
			"location":         map[string]string{"type": "string"},
			"status":           map[string]string{"type": "string", "description": "e.g. confirmed, tentative, cancelled"},
			"start": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"dateTime": map[string]string{"type": "string", "format": "date-time"},
					"timeZone": map[string]string{"type": "string"},
					"date":     map[string]string{"type": "string", "format": "date"},
				},
			},
			"end": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"dateTime": map[string]string{"type": "string", "format": "date-time"},
					"timeZone": map[string]string{"type": "string"},
					"date":     map[string]string{"type": "string", "format": "date"},
				},
			},
			"creator": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"email":       map[string]string{"type": "string", "format": "email"},
					"displayName": map[string]string{"type": "string"},
					"self":        map[string]string{"type": "boolean"},
				},
			},
			"organizer": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"email":       map[string]string{"type": "string", "format": "email"},
					"displayName": map[string]string{"type": "string"},
					"self":        map[string]string{"type": "boolean"},
				},
			},
			"attendees": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"email":          map[string]string{"type": "string", "format": "email"},
						"displayName":    map[string]string{"type": "string"},
						"responseStatus": map[string]string{"type": "string", "description": "e.g. accepted, declined, tentative, needsAction"},
						"self":           map[string]string{"type": "boolean"},
					},
				},
			},
		},
	},
}

func getDocs(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(schema)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
