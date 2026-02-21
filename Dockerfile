FROM golang:alpine AS build

ARG VAPID_PUBLIC_KEY
ENV VAPID_PUBLIC_KEY=${VAPID_PUBLIC_KEY}
ENV VITE_VAPID_PUBLIC_KEY=${VAPID_PUBLIC_KEY}

RUN apk add --update --no-cache git nodejs npm ca-certificates
RUN update-ca-certificates

WORKDIR /go/src/github.com/nicolasparada/nakama

COPY . .

WORKDIR /go/src/github.com/nicolasparada/nakama/web/app
RUN npm i
RUN npm run build

WORKDIR /go/src/github.com/nicolasparada/nakama

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /go/bin/nakama ./cmd/nakama

FROM scratch

ARG DISABLE_DEV_LOGIN=true
ENV DISABLE_DEV_LOGIN=$DISABLE_DEV_LOGIN

ARG EMBED_STATIC=true
ENV EMBED_STATIC=$EMBED_STATIC

ARG EXEC_SCHEMA=true
ENV EXEC_SCHEMA=$EXEC_SCHEMA

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /go/bin/nakama /usr/bin/nakama

EXPOSE 3000
ENTRYPOINT [ "nakama" ]
