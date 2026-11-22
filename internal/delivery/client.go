package delivery

import (
	"encoding/json"
)

// Client Клиент.
type Client struct {
	send chan json.RawMessage
}

// NewClient Конструктор.
func NewClient() *Client {
	return &Client{
		send: make(chan json.RawMessage),
	}
}

// Close Закрытие.
func (c *Client) Close() {
	close(c.send)
}
