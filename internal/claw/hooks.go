package claw

// Predefined lifecycle hook prompts.
const (
	hookSessionID  = "claw:hooks"
	hookAgentName  = "main"

	startupPrompt  = `You have just booted up. Read your memory and prepare for the day. If there are pending tasks or reminders, summarise them briefly.`
	shutdownPrompt = `The application is shutting down. Save any important context or notes to memory before the session ends.`
)

// FireStartupHook pushes a startup lifecycle event into the Gateway.
func FireStartupHook(gw *Gateway) {
	evt := NewEvent(EventHook, startupPrompt, "lifecycle:startup", hookSessionID, hookAgentName)
	gw.Push(evt)
}

// FireShutdownHook pushes a shutdown lifecycle event into the Gateway.
func FireShutdownHook(gw *Gateway) {
	evt := NewEvent(EventHook, shutdownPrompt, "lifecycle:shutdown", hookSessionID, hookAgentName)
	gw.Push(evt)
}
