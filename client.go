package wshub

import (
	"fmt"
	"io"
	"net/http"

	"golang.org/x/net/websocket"
)

func init() {
	defaultMarshal := websocket.Message.Marshal
	websocket.Message.Marshal = func(v interface{}) (msg []byte, payloadType byte, err error) {
		if data, ok := v.([]byte); ok && len(data) == 0 {
			return nil, websocket.PingFrame, nil
		}
		return defaultMarshal(v)
	}
}

// struct 註解
type Client struct {
	hub   *Hub
	conn  *websocket.Conn
	quite chan string
	msg   chan interface{}
}

func (c *Client) Request() *http.Request {
	return c.conn.Request()
}

func newClient(h *Hub, conn *websocket.Conn) (*Client, error) {
	c := &Client{
		hub:   h,
		conn:  conn,
		quite: make(chan string),
		msg:   make(chan interface{}, 10),
	}

	if err := c.Send(([]byte)(nil)); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) receiverRun(f func(string)) {
	for {
		select {
		case <-c.quite:
			c.Quite("receiver try quite sender")
			return
		default:
			var s string
			err := websocket.Message.Receive(c.conn, &s)
			if err == io.EOF {
				// 客端關閉連線
				c.Quite("client closed")
				return
			} else if err != nil {
				// 解析有錯
				c.hub.ErrorObserver(fmt.Errorf("wshub client: %s", err))
			} else {
				f(s)
			}
		}
	}
}

func (c *Client) senderRun(f func(interface{}) (interface{}, error)) {
	for {
		select {
		case m, ok := <-c.msg:
			if !ok {
				c.Quite("msg closed")
				return
			}
			m, e := f(m)
			if m == nil || e != nil {
				continue
			}
			switch m.(type) {
			case string, []byte:
				if err := websocket.Message.Send(c.conn, m); err != nil {
					c.hub.ErrorObserver(fmt.Errorf("wshub client: %s", err))
				}
			default:
				if err := websocket.JSON.Send(c.conn, m); err != nil {
					c.hub.ErrorObserver(fmt.Errorf("wshub client: %s", err))
				}
			}

		case <-c.quite:
			c.Quite("sender try quite receiver")
			return

		default:
		}
	}
}

func (c *Client) Send(data interface{}) error {
	select {
	case c.msg <- data:
		return nil
	default:
		return fmt.Errorf("client closed: %+v", c.Request().Header)
	}
}

func (c *Client) Quite(s string) {
	select {
	case c.quite <- s:
	default:
	}
}
