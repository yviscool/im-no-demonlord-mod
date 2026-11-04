package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// cdpClient is a minimal Chrome DevTools Protocol client over WebSocket.
type cdpClient struct {
	conn    *websocket.Conn
	onEvent func(method string, params json.RawMessage)

	mu      sync.Mutex
	nextID  int
	pending map[int]chan cdpReply
	closed  chan struct{}
	once    sync.Once
}

type cdpReply struct {
	Result json.RawMessage
	Err    error
}

type cdpMessage struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func dialCDP(ctx context.Context, wsURL string, onEvent func(string, json.RawMessage)) (*cdpClient, error) {
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(64 << 20)
	c := &cdpClient{
		conn:    conn,
		onEvent: onEvent,
		pending: make(map[int]chan cdpReply),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *cdpClient) readLoop() {
	defer c.shutdown(errors.New("连接已关闭"))
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		var msg cdpMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.ID != 0 && msg.Method == "" {
			c.mu.Lock()
			ch := c.pending[msg.ID]
			delete(c.pending, msg.ID)
			c.mu.Unlock()
			if ch != nil {
				reply := cdpReply{Result: msg.Result}
				if msg.Error != nil {
					reply.Err = fmt.Errorf("CDP 错误 %d: %s", msg.Error.Code, msg.Error.Message)
				}
				ch <- reply
				close(ch)
			}
			continue
		}
		if msg.Method != "" && c.onEvent != nil {
			c.onEvent(msg.Method, msg.Params)
		}
	}
}

func (c *cdpClient) shutdown(err error) {
	c.once.Do(func() {
		close(c.closed)
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
		c.mu.Lock()
		for id, ch := range c.pending {
			ch <- cdpReply{Err: err}
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
	})
}

func (c *cdpClient) Close() {
	c.shutdown(errors.New("客户端关闭"))
}

func (c *cdpClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-c.closed:
		return nil, errors.New("连接已关闭")
	default:
	}

	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan cdpReply, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	payload, err := json.Marshal(map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	})
	if err != nil {
		return nil, err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.conn.Write(writeCtx, websocket.MessageText, payload); err != nil {
		return nil, err
	}

	select {
	case reply := <-ch:
		return reply.Result, reply.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("连接已关闭")
	}
}

type evalOutcome struct {
	Value       any    `json:"value"`
	Description string `json:"description"`
	Exception   string `json:"exception"`
}

func (c *cdpClient) evaluate(ctx context.Context, expression string) (evalOutcome, error) {
	raw, err := c.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  false,
	})
	if err != nil {
		return evalOutcome{}, err
	}
	var res struct {
		Result struct {
			Type        string `json:"type"`
			Value       any    `json:"value"`
			Description string `json:"description"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return evalOutcome{}, err
	}
	out := evalOutcome{Value: res.Result.Value, Description: res.Result.Description}
	if res.ExceptionDetails != nil {
		out.Exception = res.ExceptionDetails.Text
		if res.ExceptionDetails.Exception != nil && res.ExceptionDetails.Exception.Description != "" {
			out.Exception = res.ExceptionDetails.Exception.Description
		}
	}
	return out, nil
}
