Kuch quick pointers to run it on Kali:

• go run server.go — starts server on :5000.
• go run client.go — client that posts geoIP-based location every 10s.
• Open http://127.0.0.1:5000/ in your browser to view the live map.

Agar chaho to main abhi:

Split the canvas into separate downloadable files and create a ZIP here.

Or produce a tiny setup.sh to build and run with Docker Compose.

git clone https://github.com/xdefult-coder/nu-loc.git
cd kali-location-tracker
go mod tidy
go run server.go
go run client.go
xdg-open http://127.0.0.1:5000/
docker build -t kali-locator .
docker run -p 5000:5000 kali-locator
docker compose up --build


git init
git add .
git commit -m "Kali Linux Location Tracker (Go)"
git branch -M main
git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO.git
git push -u origin main
