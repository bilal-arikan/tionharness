# Merkezi Agent Kataloğu

> **Özet:** Ayarlar → **Agent library**, bütün açık workspace’lerin kullanıcı ve sistem agentlarını ortak katalogda toplar. Profil ayarları merkezidir; workspace atamaları, arşiv durumu ve sohbet kimlikleri yereldir.

## Kullanım

- Arama ve tür/workspace filtreleriyle agent bulunur. Aynı isimdeki bağımsız agentlar birleştirilmez.
- **New agent** merkezi, henüz atanmamış bir profil oluşturur.
- Workspace düğmeleri özel agentları atar veya atamalarını kaldırır. Yerleşik sistem agentları tüm workspace’lerde zorunlu olarak bulunur.
- Merkezi ekrandaki ve workspace ekranındaki profil, araç erişimi ve varsayılana dönme işlemleri aynı merkezi kaydı değiştirir.
- Bir değişiklik birden fazla workspace’i etkiliyorsa sunucu önce onay ister. Pencere etkilenen workspace adlarını gösterir. Ebeveyn profilden kalıtım alan agentların workspace’leri de hesaba katılır.
- Atama kaldırılınca geçmiş sohbetler ve görev kayıtları tutulur; kaldırılan atama çalıştırılamaz. Yeniden atama aynı yerel agent kimliğini kullanır.
- Erişilemeyen workspace’ler atama listesinde kullanılamaz olarak gösterilir; onların okunamayan agentları listelenemez.

## Saklama ve geçiş

Merkezi profiller uygulama veri dizinindeki `agent-catalog/agents/` altında saklanır. Yerleşik agent özelleştirmeleri mevcut `system-agents.json` katmanını kullanmaya devam eder. Workspace kayıtlarındaki `catalogId` merkezi profili gösterir. Mevcut yerel ID’ler değişmez; eski sohbetlerin referansları korunur.

İlk açılışta mevcut kayıtlar merkeze aktarılır. Kaynak workspace/agent bilgisi aktarımı tekrar çalıştırılabilir yapar. Sonraki açılışlarda workspace’in eski profil kopyası merkezi ayarların üzerine yazılmaz. Merkezdeki ebeveyn/override yapısı korunur; ebeveyn başka bir workspace’te olsa da kalıtım çalışır.

Merkezi veri dizini taşınırken veya elle yedeklenirken `agent-catalog/` ve `system-agents.json` birlikte korunmalıdır. Yalnız workspace klasörünü taşımak merkezi profil verilerini içermez. Merkezi karşılığı bulunmayan bir bağlantı, başka bir agentla sessizce eşleştirilmek yerine açılış hatası verir.

## Doğrulama

Veri katmanı testleri ayrı profillerin korunmasını, workspace üzerinden ortak düzenlemeyi, araç ayarlarını, kalıtımı, yeniden atamayı, yeniden açılışı ve yerleşik tanımların tekilleştirilmesini kapsar. API testleri onaysız çoklu-workspace yazımının engellenmesini denetler. Arayüz testleri birleşik listeyi, çoklu seçimi ve onay/iptal davranışını doğrular.
