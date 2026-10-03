# Tarihsel araçlar

Bu klasör günlük geliştirme komutlarını değil, tamamlanmış deney ve tek seferlik
onarım araçlarını saklar. Kaynaklar yeniden ihtiyaç olduğunda incelenebilir;
arşivleme sırasında herhangi bir göç veya veri onarımı çalıştırılmadı.

- [52-gateway](52-gateway/README.md): gateway prototipi ve eski TS kurulumunu
  dönüştürme/uygulama yardımcıları. Çalıştırma yolları kendi rehberindedir.
- [repair-encoding.ps1](repair-encoding.ps1): eski CP1254/UTF-8 mojibake kayıtlarını
  onarma aracı. Varsayılan dry-run yalnız raporlar; `-Apply` önce `.bak-encfix`
  yedeği alır, ardından değişiklikleri yazar. Varsayılan veri kökü kullanıcı
  profilindeki `.tionharness/workspaces` dizinidir; depo konumuna bağlı değildir.

Encoding onarımını depo kökünden PowerShell ile çağırma örnekleri:

```powershell
& ./_Docs/arsiv/araclar/repair-encoding.ps1 -Workspace WS5
& ./_Docs/arsiv/araclar/repair-encoding.ps1 -StoreRoot 'C:/Users/<user>/.tionharness/workspaces' -Workspace WS5 -Apply
```

Dry-run sonucu hedeflenen eski veri sorununu gösterdiğinde uygulama örneği
kullanılır. Güncel temiz UTF-8 kayıtları için rutin bir bakım adımı değildir.
