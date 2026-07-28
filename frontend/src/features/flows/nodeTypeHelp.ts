import type { FlowNodeType } from '@/types'

// Short, user-facing explanation of every flow node type, shown by the (ⓘ)
// button next to each palette entry. Wording mirrors the engine semantics in
// internal/orchestration/model.go — keep the two in sync when node behaviour
// changes.
export const NODE_TYPE_HELP: Record<FlowNodeType, string> = {
  start:
    'Akışın zorunlu giriş noktası. Çalıştırma bu düğümden başlar ve verilen girdi ({{input}}) doğrudan bir sonraki düğüme geçer. Her akışta tam olarak bir tane bulunur.',
  end:
    'Opsiyonel bitiş düğümü. Akışı sonlandırır; istenirse bir şablonla nihai çıktıyı biçimlendirir veya bir JSON şemasıyla doğrular. Yoksa akış son düğümün çıktısıyla biter.',
  agent:
    'Seçilen ajanı, şablonlanmış bir prompt ile çalıştırır ve yanıtını çıktı yapar. Prompt içinde {{input}}, {{last}}, {{node.<id>}} gibi değişkenler kullanılabilir. Tek LLM adımı üreten temel düğüm.',
  branch:
    'Son çıktıya bakarak akışı dallara yönlendirir. Eşleşme kipine göre (içerir / regex / tam eşleşme…) ilk uyan dal seçilir; hiçbiri uymazsa varsayılan dal çalışır. LLM çağrısı yapmaz.',
  parallel:
    'Birden fazla ajan düğümünü aynı anda çalıştırır, hepsi bitince sonuçları toplayıp tek bir birleşik çıktı olarak devam düğümüne verir. Bağımsız işleri hızlandırmak için kullanılır.',
  delay:
    'Belirtilen süre kadar bekler, sonra bir sonraki düğüme geçer. LLM çağrısı yapmaz; hız sınırı, soğuma süresi veya zamanlama gerektiren adımlar için kullanılır.',
  transform:
    'LLM çağırmadan, yazdığınız şablonu render edip çıktı olarak yayar. Önceki düğümlerin sonuçlarını ({{last}}, {{node.<id>}}) birleştirmek, yeniden biçimlendirmek veya sabit metin enjekte etmek için kullanılır.',
  loop:
    'İçindeki gövde zincirini, maksimum tekrar sayısına ulaşana ya da bitiş koşulu sağlanana kadar tekrarlar. Gövde içinde {{iteration}} değişkeni geçerli tur numarasını verir.',
  'await-input':
    'Akışı kalıcı olarak duraklatır ve dışarıdan girdi bekler (insan ya da başka bir ajan besleyebilir). Girdi gelince kaldığı yerden devam eder; zaman aşımı süresi verilebilir. Sunucu yeniden başlasa bile beklemeye devam eder.',
  subflow:
    'Başka bir akışı baştan sona çalıştırır, çıktısını alır ve bu akışta devam eder. Ortak parçaları tekrar tekrar çizmek yerine yeniden kullanmayı sağlar.',
  spawn:
    'Seçilen akışları asenkron alt-koşu olarak başlatır ve BEKLEMEDEN devam eder. Sonuçlarını toplamak için ileride bir Join düğümü konur.',
  join:
    'Bariyer düğümü: bir Spawn düğümünün başlattığı alt-koşuların bitmesini bekler, çıktılarını toplar ve tek sonuç olarak devam eder.',
  coordinator:
    'Seçilen ajanı KOORDİNATÖR olarak çalıştırır: kaç worker açacağına ve onları nasıl görevlendireceğine çalışma anında kendisi karar verir — Paralel düğümün aksine genişlik tasarım anında sabit değildir. Tüm workerlar bitip koordinatör susana kadar bekler, son yanıtını bu düğümün çıktısı yapar.',
}
