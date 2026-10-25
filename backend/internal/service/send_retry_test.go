package service

import (
	"context"
	"testing"
)

func TestSavedSendIdentifiersRemainIdempotentAcrossUpgrade(t *testing.T) {
	for _, test := range []struct {
		name string
		send Send
		file bool
		hash string
	}{
		{
			name: "text",
			send: Send{ChatID: "123@s.whatsapp.net", RequestID: "legacy-text", Text: "hello"},
			hash: "452ed5f4bf7b2d601bc6027bba917aded4b84c07328d5c819f8ba321b750fa50",
		},
		{
			name: "attachment",
			send: Send{ChatID: "123@s.whatsapp.net", RequestID: "legacy-file", Data: "Zml4dHVyZQ==", Filename: "note.txt", MIME: "text/plain"},
			file: true,
			hash: "eb2145876f6be84bce346c4832c82743fd53d1207af318b0af236325320ff326",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, options := httpPairingFixture(t)
			connection, err := s.createManualConnection("Existing agent")
			if err != nil {
				t.Fatal(err)
			}
			grant := s.cfg.Grants[connection.ID]
			grant.SendEnabled = true
			s.cfg.Grants[connection.ID] = grant
			// These hashes are fixtures from the previous persisted format. A
			// retry must recognize them after restart without dispatching again.
			s.cfg.Sends[connection.ID+":"+test.send.RequestID] = sendRecord{Hash: test.hash, Status: "accepted", MessageID: "sent-before-upgrade"}
			if err := s.saveLocked(); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = New(options)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx := context.WithValue(context.Background(), identityKey{}, connection.ID)
			result, err := s.send(ctx, test.send, test.file)
			if err != nil {
				t.Fatal(err)
			}
			out := result.(map[string]any)
			if out["status"] != "accepted" || out["message_id"] != "sent-before-upgrade" {
				t.Fatalf("retry lost its saved outcome: %v", out)
			}
		})
	}
}
