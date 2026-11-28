package chatstorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/sqlite"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestCreateMessagePersistsNativeFlowWithoutChangingContent(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	ctx := whatsapp.ContextWithDevice(context.Background(), whatsapp.NewDeviceInstance("device-a", nil, nil))
	chat := types.NewJID("5571999000111", types.DefaultUserServer)
	business := types.NewJID("5571888000222", types.DefaultUserServer)
	account := types.NewJID("5571777000333", types.DefaultUserServer)
	receivedAt := time.Date(2026, time.November, 24, 12, 0, 0, 0, time.UTC)

	received := galaxyStorageFlow()
	reply := galaxyStorageReply()
	require.NoError(t, repo.CreateMessage(ctx, flowEvent("flow-1", chat, business, false, receivedAt, received)))
	require.NoError(t, repo.CreateMessage(ctx, flowEvent("flow-reply", chat, account, true, receivedAt.Add(time.Minute), reply)))
	require.NoError(t, repo.CreateMessage(ctx, flowEvent("plain-1", chat, business, false, receivedAt.Add(2*time.Minute), &waE2E.Message{
		Conversation: proto.String("bom dia"),
	})))

	stored, err := repo.GetMessageByIDChatAndDevice("device-a", chat.String(), "flow-1")
	require.NoError(t, err)
	require.Equal(t, "[galaxy_message]", stored.Content)
	require.False(t, stored.IsFromMe)
	require.JSONEq(t, utils.NativeFlowJSON(received), stored.NativeFlow)
	assertRawFlowParams(t, stored.NativeFlow)

	replyRow, err := repo.GetMessageByIDChatAndDevice("device-a", chat.String(), "flow-reply")
	require.NoError(t, err)
	require.Equal(t, "Sent", replyRow.Content)
	require.True(t, replyRow.IsFromMe)
	require.JSONEq(t, `{"replied_to_id":"flow-1"}`, replyRow.ContextMetadata)
	require.JSONEq(t, utils.NativeFlowJSON(reply), replyRow.NativeFlow)
	var decoded utils.NativeFlowPayload
	require.NoError(t, json.Unmarshal([]byte(replyRow.NativeFlow), &decoded))
	require.Equal(t, "galaxy_message", decoded.Name)
	require.Equal(t, "flow-1", decoded.RepliedToID)
	require.NotNil(t, decoded.Version)
	require.Equal(t, int32(3), *decoded.Version)

	plain, err := repo.GetMessageByIDChatAndDevice("device-a", chat.String(), "plain-1")
	require.NoError(t, err)
	require.Equal(t, "bom dia", plain.Content)
	require.Empty(t, plain.NativeFlow)

	// A later write that does not repeat the capture must not erase it.
	stored.NativeFlow = ""
	require.NoError(t, repo.StoreMessage(stored))
	kept, err := repo.GetMessageByIDAndDevice("device-a", "flow-1")
	require.NoError(t, err)
	require.Equal(t, "[galaxy_message]", kept.Content)
	require.JSONEq(t, utils.NativeFlowJSON(received), kept.NativeFlow)

	require.NoError(t, repo.StoreMessage(&domainChatStorage.Message{
		ID: "flow-1", ChatJID: chat.String(), DeviceID: "device-b", Sender: business.String(),
		Content: "[galaxy_message]", Timestamp: receivedAt,
	}))
	other, err := repo.GetMessageByIDAndDevice("device-b", "flow-1")
	require.NoError(t, err)
	require.Empty(t, other.NativeFlow)

	messages, err := repo.GetMessages(&domainChatStorage.MessageFilter{DeviceID: "device-a", ChatJID: chat.String(), Limit: 10})
	require.NoError(t, err)
	require.Len(t, messages, 3)
	byID := map[string]*domainChatStorage.Message{}
	for _, message := range messages {
		byID[message.ID] = message
	}
	require.JSONEq(t, utils.NativeFlowJSON(received), byID["flow-1"].NativeFlow)
	require.JSONEq(t, utils.NativeFlowJSON(reply), byID["flow-reply"].NativeFlow)
	require.Empty(t, byID["plain-1"].NativeFlow)

	found, err := repo.SearchMessages("device-a", chat.String(), "galaxy", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "flow-1", found[0].ID)
	require.JSONEq(t, utils.NativeFlowJSON(received), found[0].NativeFlow)

	for name, read := range map[string]func() (*domainChatStorage.Message, error){
		"global ID": func() (*domainChatStorage.Message, error) { return repo.GetMessageByID("flow-reply") },
		"device ID": func() (*domainChatStorage.Message, error) {
			return repo.GetMessageByIDAndDevice("device-a", "flow-reply")
		},
		"chat ID": func() (*domainChatStorage.Message, error) {
			return repo.GetMessageByIDChatAndDevice("device-a", chat.String(), "flow-reply")
		},
		"oldest": func() (*domainChatStorage.Message, error) {
			return repo.GetOldestMessageByDevice("device-a", chat.String())
		},
		"edit lookup": func() (*domainChatStorage.Message, error) {
			return repo.getMessageByDeviceAndChatIDAndMessageID("device-a", chat.String(), "flow-1")
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := read()
			require.NoError(t, err)
			require.NotNil(t, got)
			require.NotEmpty(t, got.NativeFlow)
		})
	}
}

func TestNativeFlowMigrationFromExistingDatabase(t *testing.T) {
	db, err := sql.Open(sqlite.DriverName, filepath.Join(t.TempDir(), "legacy.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &SQLiteRepository{db: db}
	_, err = repo.getSchemaVersion()
	require.NoError(t, err)
	migrations := repo.getMigrations()
	require.GreaterOrEqual(t, len(migrations), 50)
	for i, migration := range migrations[:len(migrations)-1] {
		require.NoError(t, repo.runMigration(migration, i+1))
	}
	_, err = db.Exec(`INSERT INTO messages (
		id, chat_jid, device_id, sender, content, timestamp, is_from_me,
		media_type, call_metadata, filename, url, direct_path, file_length, referral_metadata, context_metadata
	) VALUES ('legacy', 'chat@s.whatsapp.net', 'device-a', 'sender', '[galaxy_message]', ?, FALSE,
		'', '', '', '', '', 0, '', '')`, time.Now().UTC())
	require.NoError(t, err)

	require.NoError(t, repo.InitializeSchema())
	require.NoError(t, repo.InitializeSchema())
	version, err := repo.getSchemaVersion()
	require.NoError(t, err)
	require.Equal(t, len(migrations), version)

	legacy, err := repo.GetMessageByIDAndDevice("device-a", "legacy")
	require.NoError(t, err)
	require.Equal(t, "[galaxy_message]", legacy.Content)
	require.Empty(t, legacy.NativeFlow)

	fresh := galaxyStorageFlow()
	require.NoError(t, repo.StoreMessage(&domainChatStorage.Message{
		ID: "fresh", ChatJID: "chat@s.whatsapp.net", DeviceID: "device-a", Sender: "sender",
		Content: "[galaxy_message]", Timestamp: time.Now().UTC(), NativeFlow: utils.NativeFlowJSON(fresh),
	}))
	got, err := repo.GetMessageByIDAndDevice("device-a", "fresh")
	require.NoError(t, err)
	require.JSONEq(t, utils.NativeFlowJSON(fresh), got.NativeFlow)
	require.Equal(t, "[galaxy_message]", got.Content)
}

func flowEvent(id string, chat, sender types.JID, fromMe bool, at time.Time, msg *waE2E.Message) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsFromMe: fromMe},
			ID:            id,
			Timestamp:     at,
		},
		Message: msg,
	}
}

func galaxyStorageFlow() *waE2E.Message {
	return &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
			MessageParamsJSON: proto.String(`{"flow_id":"999000111","flow_cta":"Informar dados"}`),
			Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
				Name:             proto.String("galaxy_message"),
				ButtonParamsJSON: proto.String(`{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados","flow_action":"navigate"}`),
			}},
		}},
	}}
}

func galaxyStorageReply() *waE2E.Message {
	return &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
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
}

func assertRawFlowParams(t *testing.T, raw string) {
	t.Helper()
	var payload utils.NativeFlowPayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	require.Equal(t, "message", payload.Kind)
	require.Equal(t, `{"flow_id":"999000111","flow_cta":"Informar dados"}`, payload.MessageParamsJSON)
	require.Len(t, payload.Buttons, 1)
	require.Equal(t, "galaxy_message", payload.Buttons[0].Name)
	require.Equal(t, `{"flow_id":"999000111","flow_token":"tok_exemplo","flow_cta":"Informar dados","flow_action":"navigate"}`, payload.Buttons[0].ButtonParamsJSON)
}
