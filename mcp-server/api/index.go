package handler

import (
	"context"
	"encoding/json"
	"fmt"
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

// --- MongoDB types ---

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
	GoogleID         string     `bson:"id,omitempty" json:"id,omitempty"`
	RecurringEventID string     `bson:"recurringEventId,omitempty" json:"recurringEventId,omitempty"`
	HtmlLink         string     `bson:"htmlLink,omitempty" json:"htmlLink,omitempty"`
	HangoutLink      string     `bson:"hangoutLink,omitempty" json:"hangoutLink,omitempty"`
	Summary          string     `bson:"summary,omitempty" json:"summary,omitempty"`
	Description      string     `bson:"description,omitempty" json:"description,omitempty"`
	Location         string     `bson:"location,omitempty" json:"location,omitempty"`
	Start            EventTime  `bson:"start,omitempty" json:"start,omitempty"`
	End              EventTime  `bson:"end,omitempty" json:"end,omitempty"`
	Status           string     `bson:"status,omitempty" json:"status,omitempty"`
	Creator          Person     `bson:"creator,omitempty" json:"creator,omitempty"`
	Organizer        Person     `bson:"organizer,omitempty" json:"organizer,omitempty"`
	Attendees        []Attendee `bson:"attendees,omitempty" json:"attendees,omitempty"`
}

// --- MongoDB connection ---

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

// --- MCP protocol types ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP tool definition ---

var searchEventsTool = map[string]interface{}{
	"name":        "search_events",
	"description": "Search calendar events within a date range",
	"inputSchema": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"start_date": map[string]string{
				"type":        "string",
				"description": "Start date in YYYY-MM-DD format (inclusive)",
			},
			"end_date": map[string]string{
				"type":        "string",
				"description": "End date in YYYY-MM-DD format (inclusive)",
			},
		},
		"required": []string{"start_date", "end_date"},
	},
}

// --- Handler ---

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	token := os.Getenv("API_TOKEN")
	if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, nil, -32700, "parse error")
		return
	}

	// Notifications (no id) get 202 with no body
	if req.ID == nil && req.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "initialize":
		writeRPCResult(w, req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]string{"name": "calendar-mcp", "version": "1.0.0"},
		})

	case "tools/list":
		writeRPCResult(w, req.ID, map[string]interface{}{
			"tools": []interface{}{searchEventsTool},
		})

	case "tools/call":
		handleToolCall(w, req)

	default:
		writeRPCError(w, req.ID, -32601, "method not found")
	}
}

func handleToolCall(w http.ResponseWriter, req rpcRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeRPCError(w, req.ID, -32602, "invalid params")
		return
	}

	if params.Name != "search_events" {
		writeRPCError(w, req.ID, -32602, "unknown tool")
		return
	}

	startDate, _ := params.Arguments["start_date"].(string)
	endDate, _ := params.Arguments["end_date"].(string)
	if startDate == "" || endDate == "" {
		writeRPCError(w, req.ID, -32602, "start_date and end_date are required")
		return
	}

	events, err := searchEvents(startDate, endDate)
	if err != nil {
		writeRPCError(w, req.ID, -32603, fmt.Sprintf("internal error: %s", err.Error()))
		return
	}

	body, _ := json.MarshalIndent(events, "", "  ")
	writeRPCResult(w, req.ID, map[string]interface{}{
		"content": []map[string]string{
			{"type": "text", "text": string(body)},
		},
	})
}

func searchEvents(startDate, endDate string) ([]Event, error) {
	c, err := getCollection()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Match events that start within the date range (handles both timed and all-day events)
	filter := bson.M{
		"$or": []bson.M{
			{
				"start.dateTime": bson.M{
					"$gte": startDate + "T00:00:00",
					"$lte": endDate + "T23:59:59",
				},
			},
			{
				"start.date": bson.M{
					"$gte": startDate,
					"$lte": endDate,
				},
			},
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "start.dateTime", Value: 1}, {Key: "start.date", Value: 1}})
	cursor, err := c.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	if events == nil {
		events = []Event{}
	}
	return events, nil
}

func writeRPCResult(w http.ResponseWriter, id interface{}, result interface{}) {
	json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, id interface{}, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
}
