package registry

import (
	"encoding/json"

	"cc-control/internal/core"
)

// CPNotifier implements InstallNotifier by routing install_skill_request
// envelopes through the existing control-plane agent connection map.
type CPNotifier struct {
	CP *core.ControlPlane
}

// NotifyInstall serialises the StoredSkill as the envelope body and sends it
// to the named agent. Errors when the agent isn't currently connected.
func (n *CPNotifier) NotifyInstall(targetAgentID string, sk StoredSkill) error {
	body, err := json.Marshal(sk)
	if err != nil {
		return err
	}
	env := core.NewEnvelope("install_skill_request", targetAgentID, "")
	env.Data = body
	return n.CP.SendToAgent(targetAgentID, env)
}
