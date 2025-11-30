# Project: Kali-only Location Tracker (Go)

This single file contains a multi-file project laid out so you can copy each section into separate files. Files included:
- server.go
- client.go
- viewer.html
- go.mod
- Dockerfile
- docker-compose.yml
- README.md

---

### server.go
```go
package main

// server.go
// - HTTP API to receive location POSTs
// - Broadcasts live updates over WebSocket to connected viewers
// - Serves embedded map viewer (viewer.html)

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Location represents a single location update
type Location struct {
	Phone string  `json:"phone"`
	Token string  `json:"token,omitempty"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	IP    string  `json:"ip,omitempty"`
	When  string  `json:"when"`
}

var (
	// In-memory storage guarded by mutex for demo purposes
	store   = map[string][]Location{}
	stMutex = sync.RWMutex{}

	// WebSocket clients
	clients   = make(map[*websocket.Conn]bool)
	clientsMu = sync.Mutex{}
	upgrader  = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
)

func main() {
	r := mux.NewRouter()

	// API endpoints
	r.HandleFunc("/report", reportHandler).Methods("POST")
	r.HandleFunc("/get/{phone}", getHandler).Methods("GET")

	// Websocket for live updates
	r.HandleFunc("/ws", wsHandler)

	// Serve viewer.html and static assets
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "./viewer.html") })

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting server on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

// reportHandler accepts JSON body with phone, lat, lon, token(optional)
func reportHandler(w http.ResponseWriter, r *http.Request) {
	var loc Location
	if err := json.NewDecoder(r.Body).Decode(&loc); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	loc.When = fmt.Sprintf("%s", r.Header.Get("Date"))
	if loc.When == "" {
		loc.When = fmt.Sprintf("%v", nowISO())
	}

	// Store
	stMutex.Lock()
	store[loc.Phone] = append(store[loc.Phone], loc)
	if len(store[loc.Phone]) > 200 {
		store[loc.Phone] = store[loc.Phone][len(store[loc.Phone])-200:]
	}
	stMutex.Unlock()

	// Broadcast to websocket clients
	broadcast(loc)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	phone := vars["phone"]

	stMutex.RLock()
	locs := store[phone]
	stMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"phone": phone, "locations": locs})
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ws upgrade:", err)
		return
	}
	defer conn.Close()

	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	// Keep connection open
	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			// client likely disconnected
			clientsMu.Lock()
			delete(clients, conn)
			clientsMu.Unlock()
			break
		}
	}
}

func broadcast(loc Location) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for c := range clients {
		if err := c.WriteJSON(loc); err != nil {
			log.Println("ws write err:", err)
			c.Close()
			delete(clients, c)
		}
	}
}

func nowISO() string {
	return (time.Now()).Format(time.RFC3339)
}
```

---

### client.go
```go
package main

// client.go
// - Periodically fetches IP-based geolocation (ipinfo.io)
// - POSTs JSON to /report on the server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

type GeoIP struct {
	IP      string `json:"ip"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Loc     string `json:"loc"`
}

type Payload struct {
	Phone string  `json:"phone"`
	Token string  `json:"token,omitempty"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	IP    string  `json:"ip,omitempty"`
}

func main() {
	server := os.Getenv("SERVER_URL") // e.g. http://127.0.0.1:5000
	if server == "" {
		server = "http://127.0.0.1:5000"
	}
	phone := os.Getenv("DEVICE_PHONE")
	if phone == "" {
		phone = "kali-device"
	}
	token := os.Getenv("DEVICE_TOKEN")
	if token == "" {
		token = "mytoken123"
	}

	for {
		geo, lat, lon, err := fetchGeoIP()
		if err != nil {
			log.Println("geoip err:", err)
			time.Sleep(10 * time.Second)
			continue
		}

		p := Payload{Phone: phone, Token: token, Lat: lat, Lon: lon, IP: geo.IP}
		b, _ := json.Marshal(p)
		resp, err := http.Post(server+"/report", "application/json", bytes.NewBuffer(b))
		if err != nil {
			log.Println("post err:", err)
		} else {
			body, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Println("posted:", string(body))
		}
		time.Sleep(10 * time.Second)
	}
}

func fetchGeoIP() (GeoIP, float64, float64, error) {
	resp, err := http.Get("https://ipinfo.io/json")
	if err != nil {
		return GeoIP{}, 0, 0, err
	}
	defer resp.Body.Close()
	b, _ := ioutil.ReadAll(resp.Body)
	var g GeoIP
	json.Unmarshal(b, &g)
	var lat, lon float64
	fmt.Sscanf(g.Loc, "%f,%f", &lat, &lon)
	return g, lat, lon, nil
}
```

---

### viewer.html
```html
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>Live Viewer</title>
<link rel="stylesheet" href="https://unpkg.com/leaflet/dist/leaflet.css" />
<style>body{margin:0} #map{height:100vh}</style>
</head>
<body>
<div id="map"></div>
<script src="https://unpkg.com/leaflet/dist/leaflet.js"></script>
<script>
const map = L.map('map').setView([20.5937,78.9629],5);
L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',{maxZoom:19}).addTo(map);
let poly = L.polyline([], {weight:3}).addTo(map);
let marker = null;

const phone = new URLSearchParams(location.search).get('phone') || 'kali-device';
const token = new URLSearchParams(location.search).get('token') || '';

async function loadHistory(){
  const resp = await fetch('/get/'+encodeURIComponent(phone));
  if(!resp.ok){console.error('history fetch failed'); return}
  const json = await resp.json();
  const locs = json.locations || [];
  poly.setLatLngs(locs.map(l=>[l.lat,l.lon]));
  if(marker) map.removeLayer(marker);
  if(locs.length){
    marker = L.marker([locs[0].lat, locs[0].lon]).addTo(map);
    map.fitBounds(poly.getBounds().pad(0.5));
  }
}
loadHistory();

// WebSocket for live updates
const wsProto = (location.protocol === 'https:') ? 'wss' : 'ws';
const ws = new WebSocket(wsProto + '://' + location.host + '/ws');
ws.onmessage = (ev)=>{
  const loc = JSON.parse(ev.data);
  // only display updates for our phone
  if(loc.phone !== phone) return;
  poly.addLatLng([loc.lat, loc.lon]);
  if(marker) map.removeLayer(marker);
  marker = L.marker([loc.lat, loc.lon]).addTo(map);
};
</script>
</body>
</html>
```

---

### go.mod
```text
module locationshare

go 1.20

require github.com/gorilla/mux v1.8.0
require github.com/gorilla/websocket v1.5.0
```

---

### Dockerfile
```dockerfile
FROM golang:1.20-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o /kali-tracker ./server.go ./client.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=build /kali-tracker /usr/local/bin/kali-tracker
COPY viewer.html /viewer.html
COPY static/ /static/
EXPOSE 5000
ENTRYPOINT ["/usr/local/bin/kali-tracker"]
```

---

### docker-compose.yml
```yaml
version: '3.8'
services:
  web:
    build: .
    ports:
      - "5000:5000"
    environment:
      - PORT=5000
```

---

### README.md
```markdown
# Kali-only Location Tracker (Go)

This project runs entirely on **Kali Linux**. It uses IP-based geolocation (ipinfo.io) to produce coordinates for the Kali machine and demonstrates:
- HTTP server that accepts location reports
- WebSocket-based live broadcast for viewers
- Simple web map viewer (Leaflet)

## Quick start (local build)
1. Install Go 1.20+
2. `git clone` this repo
3. `go mod download`
4. Build and run server:
   - `go run server.go`
5. In another shell run client:
   - `go run client.go`
6. Open browser to `http://127.0.0.1:5000/` to see viewer

## Docker
Build & run with docker-compose:
```
docker compose up --build
```

## Notes
- This is a demo for study purposes only. Do not use to track other people without consent.
- For production: add authentication, HTTPS, persistent DB, and rate limits.
```

---

End of file. Copy each code block to its respective file name and run as instructed in the README.
