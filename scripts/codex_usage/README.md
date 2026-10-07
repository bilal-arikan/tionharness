# Codex kullanım kaydı onarımı

`repair-codex-usage.py`, devam ettirilen bir Codex oturumunun kümülatif
sayaçlarının her turda yeniden toplanması nedeniyle şişen eski kayıtları onarır.
Yeni sağlayıcı kodu yalnız ilgili denemede rollout dosyasına eklenen model
çağrılarını sayar; bu betik eski veriler içindir.

## Çalıştırma

Python 3 ve bash kullanılır (Windows'ta Git Bash). Önce `--apply` olmadan
çalıştırılır:

```bash
python3 scripts/repair-codex-usage.py \
  --data-dir "${TIONHARNESS_DATA_DIR:-$HOME/.tionharness}" \
  --workspace WS30 --session SES4 \
  --rollout "$ROLLOUT_PATH"
```

Windows'ta (Git Bash) Python genelde `python` adıyla gelir ve varsayılan veri dizini
`"$USERPROFILE/.tionharness"` olarak verilebilir.

`ROLLOUT_PATH`, ilgili `rollout-*.jsonl` dosyasının tam yoludur. Betik,
rollout kimliğinin oturumdaki `cliSessionId` ile aynı olduğunu doğrular.

Yazmak için TionHarness durdurulur ve aynı komuta `--apply` eklenir. Betik
uygulamanın kendi `instance.lock` dosyasını münhasır kilitler; uygulama
çalışırken yazmayı reddeder. Orijinaller veri dizininin
`backups/codex-usage-<zaman>/` altına alınır. Her kayıt geçici dosyadan atomik
olarak değiştirilir; işlem sırasında hata çıkarsa orijinaller geri yüklenir.

Onarılan kayıtlar:

- Asistan mesajlarının yalnız kullanım alanları.
- Oturum toplamları ve model çağrısı sayısı.
- İlgili günlerin ajan toplamları; diğer oturumların kullanımı korunur.
- Sohbetin model çağrısı hata ayıklama kayıtları.

Tamamlanmamış rollout, kimlik veya toplam uyuşmazlığı ve kısmen onarılmış
kümülatif kayıtlar için betik tahmin yapmak yerine durur. Türkiye için gün
sınırı varsayılan olarak UTC+3'tür; farklı geçmiş kayıtlar için
`--timezone-hours` kullanılabilir. Tamamlanmış onarım tekrar çalıştırıldığında
değişiklik yapmaz.

## Eski dosya ekleri

Eski `update_artifact` çağrıları, `file` türündeki belgenin yeni içeriğini
metadata içine kaydedip gösterilen dosya kopyasını eski sürümde bırakabiliyordu.
`--repair-artifacts`, bu oturuma ait `.md`, `.txt` ve `.html` kopyalarını son
kaydedilmiş içerikten yeniler ve artık kullanılmayan inline içeriği kaldırır.
Dosya başka bir ek tarafından paylaşılıyorsa veya oturumun sahip olduğu klasör
dışındaysa yazmayı reddeder. Proje dosyalarını değiştirmez. Bu seçenek de önce
`--apply` olmadan incelenebilir.

## Doğrulama

```bash
cd scripts
PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s codex_usage -p 'test_*.py' -v
```

Testler geçici dizinlerde çalışır; canlı veri, yedekleme, tekrar çalıştırma ve
başka oturumlara ait kullanımın korunması için gerçek veri gerekmez.
