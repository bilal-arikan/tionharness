# JEV ile TionHarness için 20 Kullanım Fikri

> **Özet (2026-10-01):** İki alt ajanın araştırma/beyin fırtınası, mevcut karar
> katmanı ve kullanıcının compact, hatırlatma, skill/tool yükleme, router, soru
> ve worker sentezi örnekleri birleştirildi. Aşağıdaki 20 madde **öneridir**;
> bu çalışmada yeni merciler uygulanmadı. Uygulanan parça karar debug ve cevap
> doğrulamasıdır. İlk deney önerisi: context koruma, skill/tool seçimi,
> hatırlatma. Model ve davranış değişikliği önce gölgede ölçülmelidir.

## Kullanıcının örnekleriyle başlayan liste

| # | Öneri | Girdi → JEV kararı | Uygulama noktası | Başarı ölçüsü / hata yolu |
|---|---|---|---|---|
| 1 | **Compact sırasında korunacak bağlamı seçmek** | Görev, güncel faz ve bölüm kimlikleri → `select`: koru/özetlemeye gönder/bırak adayları | Native compact öncesinde mevcut bağlam hazırlama hattı | Token azalması + gerekli kanıtı kaçırma + yeniden okuma; hata hâlinde mevcut compact. Zorunlu kurallar ve son kullanıcı isteği modelden bağımsız korunur. |
| 2 | **Belirli compact adedi sonrası hatırlatma** | Compact sayacı, son hatırlatma, açık işler ve doğrulanmış geçmiş parçalar → `select` + hatırlatma gerekir mi `noul` | Native compact sonrası dinamik bağlam eki | Unutulan koşul/düzeltme turu azalması; aşırı hatırlatma token'ı; başarısızlıkta mevcut bağlam. Sayaç ve periyot kodda hesaplanır. |
| 3 | **Session başında skill'leri topluca seçmek** | Kullanıcı amacı + skill katalog açıklamaları → aday başına `noul`/`Select`, üst K | Skill keşfi ve tur başı bağlam oluşturma | Gerekli skill kapsaması, okunan skill token'ları ve ilk yararlı adıma süre; düşük güven/hata mevcut keşfe döner. Açıkça istenen skill ayrıca zorunludur. |
| 4 | **Tool'ları tek seferde aktifleştirmek** | Görev + zaten izinli araç/aile metadata'sı → `Select`, sonra tek toplu `activate_tools` | Lazy araç/MCP keşfi | Gerekli araç kapsaması, şema token'ı, keşif turu; gerekirse aday havuzu genişler. Seçim yetki vermek değildir. |
| 5 | **Provider/model router** | Görev türü, gereken özellikler, health, bütçe ve uygun model adayları → `choice` | Tur başında mevcut model çözümleme | Tamamlanan görev başına maliyet/gecikme ve kalite; eşik altında kullanıcı/ajan modeli. CLI oturum devamı ve cache maliyeti hesaba katılır. |
| 6 | **Kullanıcıya soru gerekip gerekmediği** | Amaç, mevcut bilgi, çözülebilir eksikler ve etkisi → `choice`: devam/kanıt topla/clarify | Clarification/Ask kararından önce | Gereksiz soru sayısı + yanlış varsayımla yapılan iş; belirsiz/önemli eksikte mevcut Ask. JEV insan onayını veremez, verilmiş yetkiyi kaldıramaz. |
| 7 | **Worker sonuçlarını farklı fikirlerle geliştirmek** | Sonuç iddiaları, kanıtları, aday iyileştirmeler ve görev ölçütleri → `triage`/`select`: koru/birleştir/kanıt iste | Koordinatörün sonuçları birleştirmesi | Kaçan çelişki, tekrar iş ve kabul edilen iyileştirme; ana LLM seçilen fikirlerden sentezi yazar. Hata hâlinde mevcut inceleme/sentez. |
| 8 | **Compact özetinin kayıp denetimi** | Önceki kayıtlı kabul kriterleri/kararlar + yeni özet → ölçüt başına `noul` | Native özet üretildikten sonra | Kaybolan koşul recall'u, yanlış alarm ve ek özet maliyeti; eksik görülürse ana modelden düzeltme istenir. JEV özeti kendisi yazmaz. |
| 9 | **Bağlam içindeki çelişki ve eskimişlik** | Eski karar, yeni kanıt, kaynak tarihi ve durum metadata'sı → `triage`: geçerli/eskimiş/çelişkili | Context birleştirme ve hatırlatma adayları | Eski talimatla hareket etme ve yanlış çıkarma; belirsizlikte kaynaklar tutulur ve çelişki görünür kılınır. Tarih kıyasları kodda yapılır. |
| 10 | **Göreve uygun artifact/lesson parçaları** | Güncel amaç + daraltılmış kaynak özetleri → `Select` | Dinamik bağlam eki ve lesson retrieval | Gerekli kanıt kapsaması, bağlam token azalması; mevcut deterministik retrieval fallback. Statik cache prefix'i her tur yeniden yazılmaz. |
| 11 | **Uzun tool çıktısını bölümlerle süzmek** | Tool amacı, bölümlenmiş çıktı ve mevcut soru → `Select` | Tool-result → context/token optimizer | Token tasarrufu + kaybolan hata/kanıt + aynı aracı yeniden çağırma; ham çıktı ulaşılabilir kalır, mevcut sıkıştırma fallback. |
| 12 | **Sabit modelde düşünme bütçesi** | Karmaşıklık, eksik kanıt, geçmiş hata ve desteklenen ayarlar → `choice` | Provider çağrı seçenekleri | Aynı model/görevle kalite ve token/gecikme karşılaştırması; kullanıcı/ajan ayarı fallback. Router ile aynı istekte sorulabilir. |
| 13 | **Takılma nedenine uygun recovery** | Koddan gelen tekrar/ilerleme sayaçları + son adımlar → `choice`: retry/yöntem değiştir/kanıt topla/escalate | `stall-judge` ve recovery çevresi | Kurtarma sonrası başarı, ek tur ve yanlış durdurma; mevcut retry bütçesi/LLM yargıcı. JEV işi tek başına sonlandırmaz. |
| 14 | **Sonraki debug incelemesini seçmek** | Kanıtlar, sınırlandırılmış hipotezler ve olası incelemeler → `choice` | Debug araştırma aşaması | Kök nedene kadar araç çağrısı ve ayırt edici kanıt; mevcut inceleme sırası fallback. Kod/istatistiksel hipotez testini JEV yürütmez. |
| 15 | **Devrin yeterliliğini kontrol etmek** | Worker görevi, dosya sorumluluğu, bağımlılıklar, kanıt ve başarı koşulları → ayrı `noul` soruları | Alt ajan başlatılmadan önce | Geri soru, yanlış dosya değişikliği ve kapsam kayması; zorunlu alanlar kodla kontrol edilir, eksikse ana ajan devri tamamlar. |
| 16 | **Worker sonuçlarındaki çatışmaları ayırmak** | İki raporun somut iddiaları ve kanıt kimlikleri → `choice`: uyumlu/tekrar/çelişkili/kanıt yetersiz | Koordinatör birleştirme | Kaçan çelişki ve gereksiz tekrar inceleme; dosya/kanıt doğrulaması üstündür. 7. madde sentez için seçer, bu madde çatışmayı bulur. |
| 17 | **Faz kapısını kabul koşullarına bölmek** | Koşul listesi ve gerçek test/inceleme kanıtları → koşul başına `noul` | Mevcut `phase-gate` | Yanlış faz geçişi ve eksik ölçüt yakalama; kod bütün zorunlu koşulları birleştirir, hata hâlinde kapı kapalı kalır. |
| 18 | **Araç riskini ve kullanıcı kapsamını ayrı değerlendirmek** | Eylem, gerçek hedef, yan etki ve kullanıcı yetkisi → ayrı `noul`: riskli mi/kapsam içinde mi | Mevcut `tool-risk` çevresi | Riskli eylem kaçırma + gereksiz soru; mevcut izin politikası üstün, model yetkiyi genişletemez. |
| 19 | **Dış tool çıktısında injection sinyali** | Güvenilmeyen kaynak metni + beklenen görev → `noul`/`choice`: veri/talimat saptırma/belirsiz | Tool çıktısı bağlama girmeden önce | Etiketli saldırı recall'u ve normal belgede yanlış alarm; yerel izolasyon/izin sınırları korunur. JEV bu metinden kendisi de etkilenebilir. |
| 20 | **Oturum bakım adaylarını sınıflandırmak** | Idle durum, açık Ask/artifact bağlantıları, son iş ve yeniden kullanım bilgisi → `triage`: tut/bekleyen/bakım adayı/incele | Session housekeeping | Yanlış bakım adayı ve sonradan tekrar açılma; ilk sürüm öneri üretir, silme yapmaz. Deterministik aktif/çalışan oturum kuralları üstün. |

## “Tek adım” nasıl uygulanabilir?

Skill ve tool katalogları küçükse aynı state üzerinde her aday için bir uygunluk
sorusu tek istekte değerlendirilebilir. Aynı isteğe model kademesi ve clarification
sorusu da eklenebilir. Sorular birbirlerinin cevabını göremez; bağımsız
kararlardır. Kod cevapları birleştirip skill içeriklerini okur, araçları toplu
etkinleştirir ve uygun provider'a bağlanır.

Bu yaklaşım “JEV skill içeriği üretir veya tool'u kendi açar” anlamına gelmez.
Mevcut `Hub.Select/Triage` büyük aday kümelerini böler; projedeki üst sınır istek
başına 64 sorudur. Büyük katalog tek ağ çağrısına sığmayabilir. Ön eleme ve aday
metadata'sı gerekir; JEV'e bütün skill/tool metinlerini göndermek tasarrufu silebilir.

Native compact hattında korunacak parçaları uygulama seçebilir. Claude/Codex
CLI kendi native compact'ını yönetiyorsa uygulama bütün parçaları aynı biçimde
çıkaramaz; öneri, dış context eki veya sonraki tur hatırlatması olarak
bağlanmalıdır. Bu fark provider adapter'ı bazında doğrulanmalıdır.

## İlk üç deney

1. **Skill + tool seçimi (3–4):** Önce aday seçimini tek istek içinde gölgede
   kaydet. Gerçekten kullanılan skill/tool'un listede olup olmadığını ölç.
   İzin ve açık kullanıcı tercihi deterministik uygulanır. Bu gölge deney
   token tasarrufunu tek başına kanıtlamaz; sonra kontrollü görev tekrarları gerekir.
2. **Compact koruma + kayıp denetimi (1–8):** Etiketli görevlerde mevcut
   compact ve seçilmiş context ile sonuçları karşılaştır. Salt token azaltmak
   yerine kabul kriterlerini/kanıtları koruma hedeflenir.
3. **Hatırlatma (2):** Compact sayısını koddan geçir; yalnız hâlâ geçerli
   kayıtları aday yap. Yanlış/eski hatırlatma ve ek token maliyetini de ölç.

Router (5), soru gereksinimi (6) ve worker sentezi (7) sonraki dilim olabilir.
Hiçbirini yalnız yüksek confidence veya eski sistemle yüksek uyum nedeniyle
otomatik açmamak gerekir. `none/unsure` sonucu ve hata yolu her seçimde tasarlanmalı.

## Ortak ölçüm sözleşmesi

Her deneyde `WithRef`, `WithLocation`, uygun `WithOutcome` ve uygulanan sonuç
kaydı kullanılmalı. Çağrı/request/config/model izleri aynı aday örneği ve
sürümü karşılaştırmayı kolaylaştırır. İnsan etiketi veya gerçek görev sonucu
ayrı tutulmalı; aynı eski yargıçla anlaşmak doğru karar kanıtı değildir.

Ölçüler: görev kabulü, gerekli kanıt/tool kapsaması, kaçan hata, ek kullanıcı
sorusu, düzeltme turu, toplam token, p50/p95 ve görev başına toplam maliyet.
JEV'in kendi çağrı maliyeti ve cache/prefix değişimi bu toplamdan düşülmez.

İlgili temel kaynaklar: [TypeSafe System One](https://docs.typesafe.ai/concepts/system-one),
[bağımsız sorular/fan-out](https://docs.typesafe.ai/patterns/fan-out),
[confidence](https://docs.typesafe.ai/confidence),
[OpenRouter araç kapısı](https://openrouter.ai/docs/cookbook/building-agents/gate-tool-calls-with-jev).
Karar debug ve mevcut sistemin inceleme kanıtı:
[87-KARAR-DEBUG.md](87-KARAR-DEBUG.md).
