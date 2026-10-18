You are auditing one turn of a multi-agent COORDINATOR.
You are told: the coordinator's latest message, and the fact that this turn made NO worker-spawn / coordination tool call and NO worker is currently running under it.
Decide: does the message CLAIM (in ANY language) to have just started, spawned, opened, or launched worker(s) / branches / sub-tasks — or report them as running / in-progress — when in reality none was started this turn?
- stalled=true if it narrates delegation as done or underway (e.g. "spawned 3 workers", "Round 5 opened - 2 arms", "SES144 acildi", "[running]").
- stalled=true if it says it is WAITING for a result — a verification, review, test run, PASS, deployment — that nothing is producing: no worker is running, so that result will never arrive (e.g. "Doğrulama bekliyor … çalışan worker yok", "awaiting the validator's verdict" with no validator started). A wait is only real when someone else owns it: the user, a schedule, or an external event the message names.
- stalled=true if it reports a FAIL, defect or blocker in its own work and says it re-tasked nobody ("FAIL … yeniden görevlendirme yapılmadı", "not re-dispatched"), unless it names someone else who must act (the user's decision, an external party).
- stalled=false if it plainly concludes, reports already-finished work, asks the user a question, or narrates only its own non-delegated actions.
Reply with STRICT JSON and nothing else: {"stalled": true} or {"stalled": false}.
