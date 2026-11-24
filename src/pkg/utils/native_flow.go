package utils

import (
	"encoding/json"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

const (
	// NativeFlowKindMessage is a received native-flow message (WhatsApp Flow).
	NativeFlowKindMessage = "message"
	// NativeFlowKindResponse is a native-flow reply, including one sent from
	// the linked phone.
	NativeFlowKindResponse = "response"
)

// NativeFlowButtonPayload is one native-flow button. ButtonParamsJSON is the
// original buttonParamsJSON string, not a re-encoded object.
type NativeFlowButtonPayload struct {
	Name             string `json:"name,omitempty"`
	ButtonParamsJSON string `json:"button_params_json,omitempty"`
}

// NativeFlowPayload is the raw WhatsApp Flow capture stored in messages.native_flow
// and exposed as native_flow on webhooks and chat history. Params blobs stay the
// original strings because they can carry a flow_token and fields this server
// does not interpret. Callers must keep the value in storage and API payloads
// and must not log it.
type NativeFlowPayload struct {
	Kind string `json:"kind"`

	Header        string `json:"header,omitempty"`
	Subtitle      string `json:"subtitle,omitempty"`
	Body          string `json:"body,omitempty"`
	Footer        string `json:"footer,omitempty"`
	HeaderCaption string `json:"header_caption,omitempty"`

	MessageParamsJSON string                    `json:"message_params_json,omitempty"`
	Buttons           []NativeFlowButtonPayload `json:"buttons,omitempty"`

	Name        string `json:"name,omitempty"`
	ParamsJSON  string `json:"params_json,omitempty"`
	Version     *int32 `json:"version,omitempty"`
	RepliedToID string `json:"replied_to_id,omitempty"`
}

// ExtractNativeFlow returns the native-flow capture for msg, or nil when msg
// is not a native-flow message or reply. Ephemeral and view-once wrappers are
// unwrapped. The existing plain-text summary is left untouched.
func ExtractNativeFlow(msg *waE2E.Message) *NativeFlowPayload {
	msg = UnwrapMessage(msg)
	if msg == nil {
		return nil
	}
	if im := msg.GetInteractiveMessage(); im.GetNativeFlowMessage() != nil {
		return nativeFlowMessagePayload(msg, im)
	}
	if reply := msg.GetInteractiveResponseMessage(); reply.GetNativeFlowResponseMessage() != nil {
		return nativeFlowResponsePayload(msg, reply)
	}
	return nil
}

// NativeFlowJSON returns ExtractNativeFlow encoded as compact JSON, or "" when
// msg has no native-flow capture.
func NativeFlowJSON(msg *waE2E.Message) string {
	payload := ExtractNativeFlow(msg)
	if payload == nil {
		return ""
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func nativeFlowMessagePayload(msg *waE2E.Message, im *waE2E.InteractiveMessage) *NativeFlowPayload {
	flow := im.GetNativeFlowMessage()
	out := &NativeFlowPayload{
		Kind:              NativeFlowKindMessage,
		Body:              im.GetBody().GetText(),
		Footer:            im.GetFooter().GetText(),
		MessageParamsJSON: flow.GetMessageParamsJSON(),
	}
	if header := im.GetHeader(); header != nil {
		out.Header = header.GetTitle()
		out.Subtitle = header.GetSubtitle()
		out.HeaderCaption = interactiveHeaderMediaCaption(header)
	}
	for _, button := range flow.GetButtons() {
		if button == nil {
			continue
		}
		out.Buttons = append(out.Buttons, NativeFlowButtonPayload{
			Name:             button.GetName(),
			ButtonParamsJSON: button.GetButtonParamsJSON(),
		})
	}
	if id := ExtractContextInfo(msg).GetStanzaID(); id != "" {
		out.RepliedToID = id
	}
	return out
}

func nativeFlowResponsePayload(msg *waE2E.Message, reply *waE2E.InteractiveResponseMessage) *NativeFlowPayload {
	flow := reply.GetNativeFlowResponseMessage()
	out := &NativeFlowPayload{
		Kind:       NativeFlowKindResponse,
		Body:       reply.GetBody().GetText(),
		Name:       flow.GetName(),
		ParamsJSON: flow.GetParamsJSON(),
	}
	if flow.Version != nil {
		version := flow.GetVersion()
		out.Version = &version
	}
	if id := ExtractContextInfo(msg).GetStanzaID(); id != "" {
		out.RepliedToID = id
	}
	return out
}
