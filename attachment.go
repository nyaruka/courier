package courier

//-----------------------------------------------------------------------------
// MsgAttachmentUpdate Interface
//-----------------------------------------------------------------------------

// MsgAttachment represents an attachment update on a message
type MsgAttachment interface {
	ChannelUUID() ChannelUUID
	ChannelID() ChannelID
	ID() MsgID
	ExternalID() string
	Attachments() []string
}
