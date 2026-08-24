# 45 — Çoklu Seçim (Ctrl/Cmd+Click) ve Toplu Eylemler

> **Numara notu:** Eski adı `40-COKLU-SECIM.md` idi; `40-PLAN-MODE.md` ile numara
> çakıştığı için 2026-07-02'de **45**'e taşındı.

> Listelerde birden fazla öğeyi modifier tuşlarıyla seçip tek seferde toplu işlem
> yapma. Tamamen **frontend-only**; yeni backend yok. (2026-06-29)

## Etkileşim Modeli

Mevcut tek-tık davranışları (öğe açma/aktif etme) korunur; çoklu seçim modifier
tuşlarıyla ayrışır:

| Jest | Davranış |
|------|----------|
| Sol tık | Öğeyi aç/aktif et + seçimi tek öğeye sıfırla (anchor güncellenir) |
| Ctrl/Cmd + Tık | Öğeyi seçime ekle/çıkar (toggle), navigasyon yapma |
| Shift + Tık | Anchor'dan tıklanana kadar **görünür render sırasında** aralık seç |
| Ctrl/Cmd + Shift + Tık | Aralığı mevcut seçimle birleştir |
| "Tümü" butonu | Görünen (filtrelenmiş) tümünü seç |
| Esc | Seçimi temizle (yalnız ≥1 seçili iken bağlı) |

Çapraz-platform: Windows'ta `Ctrl`, macOS'ta `Cmd` → kod her zaman
`e.ctrlKey || e.metaKey` okur.

## Mimari

### `frontend/src/hooks/useMultiSelect.ts`
Liste-agnostik seçim çekirdeği. Yalnız seçim `Set`'ini ve shift-aralık `anchor`
ref'ini tutar; render eden liste `isSelected` ile stil verir, her satırın
`onClick`'inden `handleClick` çağırır.

- `handleClick(e, id, ordered): boolean` — modifier'ları yorumlar; `ordered` o an
  ekranda **render edilen görünür sıradır** (aralık grup sınırlarını aşabilir).
  Modifier basılıysa `true` döndürür → çağıran normal navigasyonu bastırmalı.
- `selected` / `count` / `isSelected` / `toggle` / `clear` / `selectAll(ids)` /
  `replace(ids)`.
- `Escape` global dinleyicisi yalnız `selected.size > 0` iken bağlanır.

### `frontend/src/components/common/SelectionBar.tsx`
Seçim ≥1 olunca beliren sticky toplu eylem çubuğu:
- `SelectionBar` — sayaç + "(+N filtre dışı)" ipucu + opsiyonel "Tümü" + temizle
  (X) + `children` (liste-özel eylem butonları). `count===0` ise hiç render etmez.
- `SelectionBarButton` — kompakt eylem butonu (`danger` varyantı kırmızı).
- `common/index.ts`'ten export edilir.

## Yeni Bir Listeye Ekleme (recipe)

```tsx
const sel = useMultiSelect()
const orderedIds = useMemo(() => visibleItems.map((x) => x.id), [visibleItems])

// satır onClick:
onClick={(e) => {
  if (sel.handleClick(e, item.id, orderedIds)) return  // seçim jesti
  openItem(item.id)                                     // normal davranış
}}

// satır className: sel.isSelected(item.id) ? ring-1 ring-[var(--color-accent)] : ...

// listenin altına:
<SelectionBar count={sel.count} onClear={sel.clear}
  onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}>
  <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>Sil</SelectionBarButton>
</SelectionBar>
```

## Bağlanan Listeler

| Liste | Bileşen | Normal tık | Toplu eylemler |
|-------|---------|-----------|----------------|
| Sohbet | `SessionsSidebar` | Oturumu aç | AI başlık · Sabitle · Arşivle/çıkar · Sil |
| Ajanlar | `AgentsView` | Ajanı seç | Sil |
| Board kartları | `TaskBoard` | Task detayı | Sütuna taşı · Ajan ata · Sil |
| Hafıza | `MemoryPanel` | Kartı genişlet | Sil (**yalnız modifier-click seçer**) |
| Artifact | `ArtifactsPanel` | Artifact aç | **Grup ata** (input+datalist, "Ata"/"Grupsuz") · Sil |
| Skills | `SkillsPanel` | Skill detayı | Görünürlük türü (Tam/Özet/İsim/Gizli) · **Erişim** (Kısıtla/Paylaş) · **Grup ata** (input+datalist, "Ata"/"Grupsuz") · Sil (katlanmış grupları atlar) |
| Flows | `FlowsPanel` (Akışlarım) | Flow'u aç | Çalıştır · Sil |
| Araçlar (ajan) | `AgentToolsSection` | Anında yasakla | "Seçilenleri yasakla" (tek PATCH) |
| Araçlar (workspace) | `ToolsPanel` (Ayarlar) | Detay aç | Etkinleştir · Devre dışı · NameOnly · Göster |
| Aktivite | `ExecutionsPanel` | Detay aç | Kimlikleri kopyala (salt-okunur) |

## Tasarım Notları / Kenar Durumları

- **Toplu eylemler mevcut tekil API'leri kullanır** — `Promise.all` / döngü ile;
  yeni endpoint yok. Optimistic state güncellenir, hata halinde `reload()`.
- **Filtre/arama:** Görünmeyen seçili öğeler korunur; SelectionBar "(+N filtre
  dışı)" gösterir (sessions). `orderedIds` yalnız görünürleri içerdiğinden
  shift-aralık tutarlıdır.
- **Yıkıcı eylem:** Tek `confirm("N öğe silinsin mi?")` onayı.
- **Hafıza özel durumu:** Kart-içi tıklamalar (genişlet/sil) plain-click ile
  çalışır; bu yüzden hafıza kartı **yalnız modifier-click** ile seçilir ve kart-içi
  handler'lar `if (e.ctrlKey||e.metaKey||e.shiftKey) return` ile modifier'ı yutar.
- **Araçlar select-all modeli:** Checkbox/toggle doğasına uygun; plain-click eski
  "anında yasakla" davranışını korur, modifier-click toplu seçim + tek `setAgentTools`.
- **Metin seçimi çakışması:** Liste satırları `<button>` olduğundan shift+click
  metin seçmez.
- **Skills toplu görünürlük:** `SelectionBarButton` yerine 4'lü segmented control
  (`data-testid="skills-bulk-visibility"`, tier butonları `skills-bulk-vis-<tier>`);
  seçili her skill için `api.setSkillVisibility` çağırıp `reload()` eder. Ayrıca
  her liste satırı artık tek bir **görünürlük çipi** (`VisibilityChip`,
  `VISIBILITY_TIERS` rengi/etiketi) taşır — eski dağınık "Gizli"/"NameOnly"
  rozetleri kaldırıldı, "Tam"/"Özet" dahil dört tier tek bakışta okunur.
- **Skills toplu erişim (Kısıtla/Paylaş):** SelectionBar'da iki-düğmeli segmented
  grup (`data-testid="skills-bulk-access"`; `skills-bulk-access-restrict` Kilit /
  `skills-bulk-access-share` Globe). Seçili her skill için `api.setSkillAccess(slug,
  shared)` → `PUT /api/skills/{slug}/access` (frontmatter `access: shared` yazar/
  siler). **Erişim ekseni görünürlükten bağımsızdır:** Kısıtla = yalnız atanan
  ajanlar (`Agent.Skills`), Paylaş = tüm ajanlar on-demand. Detayı tekil değiştiren
  `skill-detail-toggle-access` butonunun toplu karşılığı.
- **Skills toplu grup atama:** SelectionBar'da bir grup input'u (`data-testid=
  "skills-bulk-group-input"`, mevcut grup adları `datalist` ile önerilir) + "Ata"/
  "Grupsuz" butonu (`skills-bulk-group`); Enter da uygular. Seçili her skill için
  `api.setSkillGroup(slug, group)` → `PUT /api/skills/{slug}/group` (backend
  `Store.SetGroup`, yalnız `group` frontmatter'ını yazar, `category` alias'ını
  düşürür). Boş grup = grupsuz. İçe aktarılan bir paketi (import namespace, `_Docs\37`)
  sonradan tek başlıkta toplamak/dağıtmak için — her skill'i tek tek açmadan.

- **Artifact gruplama (Skills paritesi):** `ArtifactsPanel` artık becerilerdeki gibi
  `useGroupedList` ile `group` alanına göre kovalanır — katlanabilir grup başlıkları
  (`data-testid="artifacts-group-header"`), tümünü katla/aç (`artifacts-toggle-all`),
  ve SelectionBar'da grup input'u (`artifacts-bulk-group-input`, mevcut adlar
  `datalist` ile) + "Ata"/"Grupsuz" (`artifacts-bulk-group`); Enter da uygular.
  Seçili her artifact için `api.setArtifactGroup(id, group)` → `PUT /api/artifacts/
  {id}/group` (backend `DB.SetArtifactGroup`, yalnız `group` alanını yazar). Gruplama
  **arama+origin filtresinden sonra** çalışır (filtreli görünümde de kovalar). Grup
  başlıkları alfabetik (tr), "Grupsuz" en üstte. Detay artifact başlığında grup rozeti
  gösterilir. `Artifact.Group` first-class JSON alanı (skill'lerden farklı olarak
  frontmatter değil, entity alanı — artifact'lar dosya-tabanlı JSON entity'dir).
  Collapse durumu `tionharness.artifactsCollapsedGroups` localStorage'da kalıcı.
- **Tekil grup düzenleme (TSK44):** Toplu (bulk) ve sürükle-bırak'a ek olarak, tek
  bir artifact açıkken **detay editöründe** grubu doğrudan değiştirilebilir. Düzenle
  moduna girince başlık/kind/dil'in yanında bir grup input'u belirir
  (`data-testid="artifact-edit-group-input"`, mevcut grup adları `artifacts-group-names`
  datalist'iyle önerilir; boş = grupsuz). Kaydet, önce içerik/meta patch'ini
  (`api.updateArtifact` — bu handler `group`'u yok sayar), grup değiştiyse ardından
  `api.setArtifactGroup(id, group)` çağırır; son yanıt kaydedilmiş artifact'tir.
  `dirty` bayrağı grup farkını da içerir. Böylece kullanıcı tek bir artifact'i
  gruplamak için artık çoklu-seçime girmek zorunda değil.

## İleride
- Klavye gezinme (Space=toggle, Shift+Ok ile aralık).
- Aktivite "2 seçili → karşılaştır" (debug/usage diff) — şu an yalnız kimlik kopyalama.
- `aria-multiselectable` / `aria-selected` erişilebilirlik etiketleri.
