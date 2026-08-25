package agent

import "testing"

func TestDetectUnbackedSpawnClaim(t *testing.T) {
	tests := []struct {
		name            string
		text            string
		toolNames       []string
		coordinatorMode bool
		want            bool
	}{
		{name: "real incident worker started", text: "Uygulama worker’ı başladı. Kapsam: taslak koruma.", coordinatorMode: true, want: true},
		{name: "real incident worker was started", text: "Uygulama worker’ı başlatıldı. Sonuç gelince validator çalışacak.", coordinatorMode: true, want: true},
		{name: "real incident validator started", text: "Validator başladı. Test sonucu gelince kart kapatılacak.", coordinatorMode: true, want: true},
		{name: "real incident named validator", text: "Bağımsız validator gerçekten başladı: `/root/validator`.", coordinatorMode: true, want: true},
		{name: "english spawned", text: "Spawned two workers for the review.", coordinatorMode: true, want: true},
		{name: "english started worker", text: "Started a worker to run tests.", coordinatorMode: true, want: true},
		{name: "english spawning", text: "Spawning a validator now.", coordinatorMode: true, want: true},
		{name: "delegated", text: "İşi delege ettim.", coordinatorMode: true, want: true},
		{name: "turkish future", text: "Worker'ı birazdan başlatacağım.", coordinatorMode: true, want: false},
		{name: "turkish passive future", text: "Validator sonraki turda başlatılacak.", coordinatorMode: true, want: false},
		{name: "english future", text: "I will start a worker after approval.", coordinatorMode: true, want: false},
		{name: "question", text: "Worker başlamış olabilir mi?", coordinatorMode: true, want: false},
		{name: "unrelated tool does not prove spawn", text: "Spawned a worker.", toolNames: []string{"list_workers"}, coordinatorMode: true, want: true},
		{name: "spawn worker proves spawn", text: "Spawned a worker.", toolNames: []string{"spawn_worker"}, coordinatorMode: true, want: false},
		{name: "namespaced spawn worker proves spawn", text: "Spawned a worker.", toolNames: []string{"mcp__tionharness_interaction__spawn_worker"}, coordinatorMode: true, want: false},
		{name: "spawn worker claim in fenced code", text: "```text\nspawn_worker called. Validator başladı.\n```", coordinatorMode: true, want: false},
		{name: "inline spawn worker quote", text: "Diagnostic reference: `spawn_worker`", coordinatorMode: true, want: false},
		{name: "claim in quote", text: "> Validator başladı.\nNo action taken.", coordinatorMode: true, want: false},
		{name: "turkish negative", text: "Worker başlatmadım.", coordinatorMode: true, want: false},
		{name: "turkish negative with context", text: "Bu turda worker başlatmadım, sadece durumu özetliyorum.", coordinatorMode: true, want: false},
		{name: "turkish negative not yet", text: "Henüz hiçbir worker başlatmadım.", coordinatorMode: true, want: false},
		{name: "english negative", text: "I did not spawn a worker.", coordinatorMode: true, want: false},
		{name: "english negative this turn", text: "I did not spawn a worker this turn.", coordinatorMode: true, want: false},
		{name: "english passive negative", text: "No worker was started.", coordinatorMode: true, want: false},
		{name: "not coordinator", text: "Spawned a worker.", coordinatorMode: false, want: false},
		{name: "empty", text: "", coordinatorMode: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectUnbackedSpawnClaim(tt.text, tt.toolNames, tt.coordinatorMode); got != tt.want {
				t.Fatalf("DetectUnbackedSpawnClaim(%q, %v, %v) = %v, want %v", tt.text, tt.toolNames, tt.coordinatorMode, got, tt.want)
			}
		})
	}
}
