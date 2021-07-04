package connectorlib

import "time"

type LuaBlock struct {
	ID      uint32    `json:"id"`
	Stamp   time.Time `json:"stamp"`
	Type    byte      `json:"type"`
	Message string    `json:"message"`
	Address uint32    `json:"address"`
	Domain  string    `json:"domain"` // "System Bus"
	Value   uint32    `json:"value"`
	Block   []byte    `json:"block"`
}
