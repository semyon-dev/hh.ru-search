FROM golang:1.15.2

RUN apt -y update && apt -y install build-essential

COPY . /
WORKDIR /

# Build the Go app
RUN go mod download
RUN go build -o search_api main.go
ENV ES_HOST="http://elasticsearch:9200"
CMD ["./search_api"]
EXPOSE 8080
