package nakama

//go:generate go tool moq -rm -stub -out ./mailing/sender_mock.go ./mailing Sender
//go:generate go tool moq -rm -stub -out ./transport/service_mock.go ./transport Service
