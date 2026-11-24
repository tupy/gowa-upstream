package whatsapp

import (
	"context"
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func TestProcessHistorySyncStoresNativeFlowForBothDirections(t *testing.T) {
	originalLog := log
	log = waLog.Noop
	t.Cleanup(func() { log = originalLog })

	chatJID := "628123456789@s.whatsapp.net"
	accountJID := types.NewADJID("628111111111", 0, 7)
	client := &whatsmeow.Client{Store: &store.Device{ID: &accountJID}}
	ctx := ContextWithDevice(context.Background(), NewDeviceInstance("device-alias", client, nil))
	received := &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
		InteractiveMessage: &waE2E.InteractiveMessage{
			InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
					Name:             proto.String("galaxy_message"),
					ButtonParamsJSON: proto.String(`{"flow_id":"999000111","flow_token":"tok_exemplo"}`),
				}},
			}},
		},
	}}}
	reply := &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
		Body:        &waE2E.InteractiveResponseMessage_Body{Text: proto.String("Sent")},
		ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("hist-flow")},
		InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
			NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
				Name:       proto.String("galaxy_message"),
				ParamsJSON: proto.String(`{"cpf":"00000000000"}`),
				Version:    proto.Int32(3),
			},
		},
	}}
	syncType := waHistorySync.HistorySync_RECENT
	data := &waHistorySync.HistorySync{
		SyncType: &syncType,
		Conversations: []*waHistorySync.Conversation{{
			ID: proto.String(chatJID),
			Messages: []*waHistorySync.HistorySyncMsg{
				historyFlowMessage("hist-flow", false, received),
				historyFlowMessage("hist-reply", true, reply),
			},
		}},
	}

	repo := &historyMessageBatchRepoSpy{}
	if err := processHistorySync(ctx, data, repo, client); err != nil {
		t.Fatalf("processHistorySync: %v", err)
	}
	if len(repo.lastBatch) != 2 {
		t.Fatalf("stored %d messages, want 2", len(repo.lastBatch))
	}
	gotReceived, gotReply := repo.lastBatch[0], repo.lastBatch[1]
	if gotReceived.Content != "[galaxy_message]" || gotReceived.IsFromMe || gotReceived.NativeFlow != utils.NativeFlowJSON(received) {
		t.Fatalf("received history flow = content %q fromMe %v flow %s", gotReceived.Content, gotReceived.IsFromMe, gotReceived.NativeFlow)
	}
	if gotReply.Content != "Sent" || !gotReply.IsFromMe || gotReply.NativeFlow != utils.NativeFlowJSON(reply) {
		t.Fatalf("reply history flow = content %q fromMe %v flow %s", gotReply.Content, gotReply.IsFromMe, gotReply.NativeFlow)
	}
	if gotReply.ContextMetadata != `{"replied_to_id":"hist-flow"}` {
		t.Fatalf("reply context = %s", gotReply.ContextMetadata)
	}
}

func historyFlowMessage(id string, fromMe bool, message *waE2E.Message) *waHistorySync.HistorySyncMsg {
	return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{
			RemoteJID: proto.String("628123456789@s.whatsapp.net"),
			FromMe:    proto.Bool(fromMe),
			ID:        proto.String(id),
		},
		Message:          message,
		MessageTimestamp: proto.Uint64(1788249600),
	}}
}
