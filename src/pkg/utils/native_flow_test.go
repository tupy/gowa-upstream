package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

const (
	flowButtonParams  = `{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados","flow_action":"navigate","note":"a&b"}`
	flowMessageParams = `{"z":1, "flow_id":"999000111","flow_cta":"Informar dados"}`
	flowReplyParams   = `{"cpf":"00000000000","unidade_consumidora":"123456789","flow_token":"tok_resposta"}`
)

func galaxyFlowMessage() *waE2E.Message {
	return &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
			MessageParamsJSON: proto.String(flowMessageParams),
			Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
				Name:             proto.String("galaxy_message"),
				ButtonParamsJSON: proto.String(flowButtonParams),
			}},
		}},
	}}
}

func richGalaxyFlowMessage() *waE2E.Message {
	msg := galaxyFlowMessage()
	im := msg.GetInteractiveMessage()
	im.Header = &waE2E.InteractiveMessage_Header{
		Title:    proto.String("Neoenergia Coelba"),
		Subtitle: proto.String("Atualização cadastral"),
		Media: &waE2E.InteractiveMessage_Header_ImageMessage{ImageMessage: &waE2E.ImageMessage{
			Caption: proto.String("foto da conta"),
		}},
	}
	im.Body = &waE2E.InteractiveMessage_Body{Text: proto.String("Toque no botão para informar seus dados.")}
	im.Footer = &waE2E.InteractiveMessage_Footer{Text: proto.String("Mensagem automática")}
	return msg
}

func galaxyFlowReply() *waE2E.Message {
	return &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
		Body: &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Sent")},
		ContextInfo: &waE2E.ContextInfo{
			StanzaID: proto.String("3EB0FORMULARIO0001"),
		},
		InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
			NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
				Name:       proto.String("galaxy_message"),
				ParamsJSON: proto.String(flowReplyParams),
				Version:    proto.Int32(3),
			},
		},
	}}
}

func TestExtractNativeFlowKeepsRawParamsAndExistingText(t *testing.T) {
	t.Run("button only flow stays [galaxy_message]", func(t *testing.T) {
		msg := galaxyFlowMessage()
		if got := ExtractMessageTextFromProto(msg); got != "[galaxy_message]" {
			t.Fatalf("content = %q, want [galaxy_message]", got)
		}
		payload := decodeNativeFlow(t, msg)
		if payload.Kind != NativeFlowKindMessage {
			t.Fatalf("kind = %q", payload.Kind)
		}
		if payload.MessageParamsJSON != flowMessageParams {
			t.Fatalf("message params = %q", payload.MessageParamsJSON)
		}
		if len(payload.Buttons) != 1 || payload.Buttons[0].Name != "galaxy_message" || payload.Buttons[0].ButtonParamsJSON != flowButtonParams {
			t.Fatalf("buttons = %+v", payload.Buttons)
		}
		if payload.Header != "" || payload.Body != "" || payload.RepliedToID != "" {
			t.Fatalf("unexpected text fields: %+v", payload)
		}
	})

	t.Run("header body and footer are captured without changing the summary", func(t *testing.T) {
		msg := richGalaxyFlowMessage()
		const wantText = "Neoenergia Coelba\nAtualização cadastral\nfoto da conta\nToque no botão para informar seus dados.\nMensagem automática\n[galaxy_message]"
		if got := ExtractMessageTextFromProto(msg); got != wantText {
			t.Fatalf("content = %q, want %q", got, wantText)
		}
		payload := decodeNativeFlow(t, msg)
		if payload.Header != "Neoenergia Coelba" || payload.Subtitle != "Atualização cadastral" || payload.HeaderCaption != "foto da conta" {
			t.Fatalf("header texts = %+v", payload)
		}
		if payload.Body != "Toque no botão para informar seus dados." || payload.Footer != "Mensagem automática" {
			t.Fatalf("body/footer = %+v", payload)
		}
		if payload.Buttons[0].ButtonParamsJSON != flowButtonParams {
			t.Fatalf("raw button params changed: %q", payload.Buttons[0].ButtonParamsJSON)
		}
	})

	t.Run("phone reply keeps Sent and the quoted form id", func(t *testing.T) {
		msg := galaxyFlowReply()
		if got := ExtractMessageTextFromProto(msg); got != "Sent" {
			t.Fatalf("content = %q, want Sent", got)
		}
		payload := decodeNativeFlow(t, msg)
		if payload.Kind != NativeFlowKindResponse || payload.Name != "galaxy_message" || payload.Body != "Sent" {
			t.Fatalf("response = %+v", payload)
		}
		if payload.ParamsJSON != flowReplyParams {
			t.Fatalf("params = %q", payload.ParamsJSON)
		}
		if payload.Version == nil || *payload.Version != 3 {
			t.Fatalf("version = %v", payload.Version)
		}
		if payload.RepliedToID != "3EB0FORMULARIO0001" {
			t.Fatalf("replied_to_id = %q", payload.RepliedToID)
		}
		const want = `{"kind":"response","body":"Sent","name":"galaxy_message","params_json":"{\"cpf\":\"00000000000\",\"unidade_consumidora\":\"123456789\",\"flow_token\":\"tok_resposta\"}","version":3,"replied_to_id":"3EB0FORMULARIO0001"}`
		if got := NativeFlowJSON(msg); got != want {
			t.Fatalf("json = %s\nwant %s", got, want)
		}
	})

	t.Run("version zero is kept and a missing version is omitted", func(t *testing.T) {
		withZero := galaxyFlowReply()
		withZero.GetInteractiveResponseMessage().GetNativeFlowResponseMessage().Version = proto.Int32(0)
		payload := decodeNativeFlow(t, withZero)
		if payload.Version == nil || *payload.Version != 0 {
			t.Fatalf("version = %v, want 0", payload.Version)
		}
		if !json.Valid([]byte(NativeFlowJSON(withZero))) || !strings.Contains(NativeFlowJSON(withZero), `"version":0`) {
			t.Fatalf("version 0 dropped from %s", NativeFlowJSON(withZero))
		}

		missing := galaxyFlowReply()
		missing.GetInteractiveResponseMessage().GetNativeFlowResponseMessage().Version = nil
		payload = decodeNativeFlow(t, missing)
		if payload.Version != nil {
			t.Fatalf("expected omitted version, got %d", *payload.Version)
		}
	})

	t.Run("wrapped reply and non-flow messages", func(t *testing.T) {
		wrapped := &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: galaxyFlowReply()}}
		if ExtractMessageTextFromProto(wrapped) != "Sent" {
			t.Fatalf("wrapped content = %q", ExtractMessageTextFromProto(wrapped))
		}
		if decodeNativeFlow(t, wrapped).RepliedToID != "3EB0FORMULARIO0001" {
			t.Fatal("wrapped reply lost the quoted form id")
		}
		if NativeFlowJSON(&waE2E.Message{Conversation: proto.String("oi")}) != "" {
			t.Fatal("plain text produced a native flow payload")
		}
		if NativeFlowJSON(&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
			Body: &waE2E.InteractiveMessage_Body{Text: proto.String("sem flow")},
		}}) != "" {
			t.Fatal("interactive message without native flow produced a payload")
		}
		if NativeFlowJSON(&waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
			Body: &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Sent")},
		}}) != "" {
			t.Fatal("response without native flow produced a payload")
		}
	})
}

func TestExtractContextInfoInteractiveResponseMessage(t *testing.T) {
	msg := galaxyFlowReply()
	ci := ExtractContextInfo(msg)
	if ci.GetStanzaID() != "3EB0FORMULARIO0001" {
		t.Fatalf("StanzaID = %q", ci.GetStanzaID())
	}
	if got := ExtractContextMetadata(msg); got != `{"replied_to_id":"3EB0FORMULARIO0001"}` {
		t.Fatalf("context metadata = %s", got)
	}
	built := BuildEventMessage(&events.Message{Info: types.MessageInfo{ID: "reply-1"}, Message: msg})
	if built.Text != "Sent" || built.RepliedId != "3EB0FORMULARIO0001" {
		t.Fatalf("event message = %+v", built)
	}

	without := &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
		Body: &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Sent")},
	}}
	if ExtractContextInfo(without) != nil {
		t.Fatal("expected nil context when the reply has none")
	}
}

func decodeNativeFlow(t *testing.T, msg *waE2E.Message) NativeFlowPayload {
	t.Helper()
	raw := NativeFlowJSON(msg)
	if raw == "" {
		t.Fatal("expected native flow JSON")
	}
	var payload NativeFlowPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal native flow: %v", err)
	}
	return payload
}
