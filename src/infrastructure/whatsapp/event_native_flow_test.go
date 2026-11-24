package whatsapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestBuildEventPayloadNativeFlowKeepsBodyAndRawParams(t *testing.T) {
	received := &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
		Header: &waE2E.InteractiveMessage_Header{Title: proto.String("Neoenergia Coelba")},
		Body:   &waE2E.InteractiveMessage_Body{Text: proto.String("Toque no botão para informar seus dados.")},
		Footer: &waE2E.InteractiveMessage_Footer{Text: proto.String("Mensagem automática")},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
			MessageParamsJSON: proto.String(`{"flow_id":"999000111","flow_cta":"Informar dados"}`),
			Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
				Name:             proto.String("galaxy_message"),
				ButtonParamsJSON: proto.String(`{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados","flow_action":"navigate"}`),
			}},
		}},
	}}
	_, payload, err := buildEventPayload(context.Background(), nil, businessEvent("FLOW1", received))
	require.NoError(t, err)
	const wantBody = "Neoenergia Coelba\nToque no botão para informar seus dados.\nMensagem automática\n[galaxy_message]"
	require.Equal(t, wantBody, payload["body"])
	require.Equal(t, wantBody, payload["interactive"])
	flow, ok := payload["native_flow"].(utils.NativeFlowPayload)
	require.True(t, ok)
	require.Equal(t, "message", flow.Kind)
	require.Equal(t, "Neoenergia Coelba", flow.Header)
	require.Equal(t, "Toque no botão para informar seus dados.", flow.Body)
	require.Equal(t, "Mensagem automática", flow.Footer)
	require.Equal(t, `{"flow_id":"999000111","flow_cta":"Informar dados"}`, flow.MessageParamsJSON)
	require.Equal(t, "galaxy_message", flow.Buttons[0].Name)
	require.Equal(t, `{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados","flow_action":"navigate"}`, flow.Buttons[0].ButtonParamsJSON)
	_, hasSelection := payload["selection"]
	require.False(t, hasSelection)

	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	var replayed map[string]any
	require.NoError(t, json.Unmarshal(raw, &replayed))
	replayedFlow, ok := replayed["native_flow"].(map[string]any)
	require.True(t, ok, "native_flow must stay an object across the webhook JSON round-trip")
	buttons := replayedFlow["buttons"].([]any)
	button := buttons[0].(map[string]any)
	require.Equal(t, `{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados","flow_action":"navigate"}`, button["button_params_json"])
	require.Equal(t, wantBody, replayed["body"])
}

func TestBuildEventPayloadNativeFlowReplyLinksQuotedForm(t *testing.T) {
	reply := &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
		Body:        &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Sent")},
		ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("3EB0FORMULARIO0001")},
		InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
			NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
				Name:       proto.String("galaxy_message"),
				ParamsJSON: proto.String(`{"cpf":"00000000000","unidade_consumidora":"123456789"}`),
				Version:    proto.Int32(3),
			},
		},
	}}
	_, payload, err := buildEventPayload(context.Background(), nil, businessEvent("FLOWREPLY", reply))
	require.NoError(t, err)
	require.Equal(t, "Sent", payload["body"])
	require.Equal(t, "3EB0FORMULARIO0001", payload["replied_to_id"])

	selection, ok := payload["selection"].(utils.BusinessSelection)
	require.True(t, ok)
	require.Equal(t, "interactive", selection.Kind)
	require.Equal(t, "Sent", selection.Text)

	flow, ok := payload["native_flow"].(utils.NativeFlowPayload)
	require.True(t, ok)
	require.Equal(t, "response", flow.Kind)
	require.Equal(t, "galaxy_message", flow.Name)
	require.Equal(t, "Sent", flow.Body)
	require.Equal(t, `{"cpf":"00000000000","unidade_consumidora":"123456789"}`, flow.ParamsJSON)
	require.NotNil(t, flow.Version)
	require.Equal(t, int32(3), *flow.Version)
	require.Equal(t, "3EB0FORMULARIO0001", flow.RepliedToID)

	_, plainPayload, err := buildEventPayload(context.Background(), nil, businessEvent("TEXT1", &waE2E.Message{Conversation: proto.String("oi")}))
	require.NoError(t, err)
	_, hasFlow := plainPayload["native_flow"]
	require.False(t, hasFlow)
}
