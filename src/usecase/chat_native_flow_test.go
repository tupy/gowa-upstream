package usecase

import (
	"encoding/json"
	"testing"
	"time"

	domainChat "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chat"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestGetChatMessagesExposesNativeFlow(t *testing.T) {
	_, repo, ctx := newMessageActionTestService(t, nil)
	chatJID := types.NewJID("5571999000111", types.DefaultUserServer)
	business := types.NewJID("5571888000222", types.DefaultUserServer)
	account := types.NewJID("5571777000333", types.DefaultUserServer)
	at := time.Date(2026, time.November, 24, 15, 0, 0, 0, time.UTC)
	received := &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
			MessageParamsJSON: proto.String(`{"flow_id":"999000111"}`),
			Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
				Name:             proto.String("galaxy_message"),
				ButtonParamsJSON: proto.String(`{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados"}`),
			}},
		}},
	}}
	reply := &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
		Body:        &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Sent")},
		ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("flow-1")},
		InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
			NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
				Name:       proto.String("galaxy_message"),
				ParamsJSON: proto.String(`{"cpf":"00000000000","unidade_consumidora":"123456789"}`),
				Version:    proto.Int32(3),
			},
		},
	}}
	require.NoError(t, repo.CreateMessage(ctx, &events.Message{
		Info:    types.MessageInfo{ID: "flow-1", Timestamp: at, MessageSource: types.MessageSource{Chat: chatJID, Sender: business}},
		Message: received,
	}))
	require.NoError(t, repo.CreateMessage(ctx, &events.Message{
		Info: types.MessageInfo{ID: "flow-reply", Timestamp: at.Add(time.Minute), MessageSource: types.MessageSource{
			Chat: chatJID, Sender: account, IsFromMe: true,
		}},
		Message: reply,
	}))
	require.NoError(t, repo.CreateMessage(ctx, &events.Message{
		Info:    types.MessageInfo{ID: "plain-1", Timestamp: at.Add(2 * time.Minute), MessageSource: types.MessageSource{Chat: chatJID, Sender: business}},
		Message: &waE2E.Message{Conversation: proto.String("bom dia")},
	}))

	fromMe := true
	for _, tc := range []struct {
		name    string
		search  string
		fromMe  *bool
		wantIDs []string
	}{
		{name: "list", wantIDs: []string{"plain-1", "flow-reply", "flow-1"}},
		{name: "search", search: "Sent", wantIDs: []string{"flow-reply"}},
		{name: "from me", fromMe: &fromMe, wantIDs: []string{"flow-reply"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := NewChatService(repo).GetChatMessages(ctx, domainChat.GetChatMessagesRequest{
				ChatJID: chatJID.String(), Limit: 20, Search: tc.search, IsFromMe: tc.fromMe,
			})
			require.NoError(t, err)
			require.Len(t, response.Data, len(tc.wantIDs))
			payload, err := json.Marshal(response.Data)
			require.NoError(t, err)
			var messages []map[string]any
			require.NoError(t, json.Unmarshal(payload, &messages))
			for i, message := range messages {
				require.Equal(t, tc.wantIDs[i], message["id"])
			}
			byID := map[string]map[string]any{}
			for _, message := range messages {
				byID[message["id"].(string)] = message
			}
			if row, ok := byID["flow-1"]; ok {
				require.Equal(t, "[galaxy_message]", row["content"])
				flow := row["native_flow"].(map[string]any)
				require.Equal(t, "message", flow["kind"])
				buttons := flow["buttons"].([]any)
				require.Equal(t, `{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados"}`, buttons[0].(map[string]any)["button_params_json"])
			}
			if row, ok := byID["flow-reply"]; ok {
				require.Equal(t, "Sent", row["content"])
				require.Equal(t, true, row["is_from_me"])
				flow := row["native_flow"].(map[string]any)
				require.Equal(t, "response", flow["kind"])
				require.Equal(t, "galaxy_message", flow["name"])
				require.Equal(t, `{"cpf":"00000000000","unidade_consumidora":"123456789"}`, flow["params_json"])
				require.Equal(t, float64(3), flow["version"])
				require.Equal(t, "flow-1", flow["replied_to_id"])
				require.Equal(t, `{"replied_to_id":"flow-1"}`, row["context_metadata"])
			}
			if row, ok := byID["plain-1"]; ok {
				require.Equal(t, "bom dia", row["content"])
				_, exists := row["native_flow"]
				require.False(t, exists)
			}
		})
	}
}
