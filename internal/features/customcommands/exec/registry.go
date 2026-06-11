package exec

import (
	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// registerStdHandlers wires every step kind to its concrete handler. New step
// kinds need a function added here AND a constant in customcommands/kinds.go
// AND a case in validate.go's validStepKind().
func registerStdHandlers(e *Engine) {
	// Reply / message surface
	e.Register(cc.KindDeferReply, hDeferReply)
	e.Register(cc.KindReply, hReply)
	e.Register(cc.KindEditReply, hEditReply)
	e.Register(cc.KindSendMessage, hSendMessage)
	e.Register(cc.KindSendDM, hSendDM)
	e.Register(cc.KindEmbedSend, hEmbedSend)
	e.Register(cc.KindModalOpen, hModalOpen)
	e.Register(cc.KindMessageEdit, hMessageEdit)
	e.Register(cc.KindMessageFetch, hMessageFetch)
	e.Register(cc.KindMessageDelete, hMessageDelete)
	e.Register(cc.KindMessagePurge, hMessagePurge)
	e.Register(cc.KindMessageCrosspost, hMessageCrosspost)
	e.Register(cc.KindReactAdd, hReactAdd)
	e.Register(cc.KindReactRemove, hReactRemove)
	e.Register(cc.KindReactClear, hReactClear)
	e.Register(cc.KindPinAdd, hPinAdd)
	e.Register(cc.KindPinRemove, hPinRemove)

	// Roles / members
	e.Register(cc.KindRoleAdd, hRoleAdd)
	e.Register(cc.KindRoleRemove, hRoleRemove)
	e.Register(cc.KindMemberNickname, hMemberNickname)
	e.Register(cc.KindMemberKick, hMemberKick)
	e.Register(cc.KindMemberBan, hMemberBan)
	e.Register(cc.KindMemberUnban, hMemberUnban)
	e.Register(cc.KindMemberTimeout, hMemberTimeout)
	e.Register(cc.KindMemberFetch, hMemberFetch)

	// Channels / threads / voice
	e.Register(cc.KindChannelCreate, hChannelCreate)
	e.Register(cc.KindChannelEdit, hChannelEdit)
	e.Register(cc.KindChannelDelete, hChannelDelete)
	e.Register(cc.KindThreadCreate, hThreadCreate)
	e.Register(cc.KindThreadArchive, hThreadArchive)
	e.Register(cc.KindThreadMember, hThreadMember)
	e.Register(cc.KindInviteCreate, hInviteCreate)
	e.Register(cc.KindVoiceMove, hVoiceMove)
	e.Register(cc.KindVoiceSet, hVoiceSet)

	// Image
	e.Register(cc.KindImageRender, hImageRender)
	e.Register(cc.KindImageAttach, hImageAttach)
	e.Register(cc.KindImageLoad, hImageLoad)

	// Data
	e.Register(cc.KindSetVar, hSetVar)
	e.Register(cc.KindIncrVar, hIncrVar)
	e.Register(cc.KindPickRandom, hPickRandom)
	e.Register(cc.KindJSONParse, hJSONParse)
	e.Register(cc.KindKVGet, hKVGet)
	e.Register(cc.KindKVSet, hKVSet)
	e.Register(cc.KindKVDelete, hKVDelete)
	e.Register(cc.KindHTTPReq, hHTTPRequest)

	// Durable
	e.Register(cc.KindWait, hWait)
	e.Register(cc.KindWaitFor, hWaitFor)

	// Terminal / misc
	e.Register(cc.KindExit, hExit)
	e.Register(cc.KindFail, hFail)
	e.Register(cc.KindNoop, hNoop)
	e.Register(cc.KindRunCommand, hRunCommand)
	e.Register(cc.KindAuditNote, hAuditNote)
}
