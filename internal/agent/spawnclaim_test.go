package agent

import "testing"

func TestDetectUnbackedSpawnClaim(t *testing.T) {
	tests := []struct {
		name            string
		text            string
		toolCallCount   int
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
		{name: "tool call present", text: "Spawned a worker.", toolCallCount: 1, coordinatorMode: true, want: false},
		{name: "not coordinator", text: "Spawned a worker.", coordinatorMode: false, want: false},
		{name: "empty", text: "", coordinatorMode: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectUnbackedSpawnClaim(tt.text, tt.toolCallCount, tt.coordinatorMode); got != tt.want {
				t.Fatalf("DetectUnbackedSpawnClaim(%q, %d, %v) = %v, want %v", tt.text, tt.toolCallCount, tt.coordinatorMode, got, tt.want)
			}
		})
	}
}
