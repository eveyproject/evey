# Google Play - éléments visuels

| Fichier | Format Play | Contrainte |
|---|---|---|
| `play-icon-512.png` | Icône de l'application | 512x512, PNG, < 1 Mo |
| `play-feature-graphic-1024x500.png` | Image de présentation | 1024x500, PNG, < 15 Mo |

Les `.svg` sont la source, les `.png` en sont le rendu. Après modification d'un SVG :

```bash
node evey-brand/play/render.mjs
```

Le rendu passe par le chromium de Playwright livré avec `tests/e2e` (pas de
cairosvg/ImageMagick sur la machine) et embarque la woff2 Inter du front pour la
baseline, sinon elle tomberait sur une police de repli.

## Choix de mise en page

- **Icône** : plein bord, aucun arrondi cuit dans l'image - Play applique son
  propre masque à coins arrondis. Le V tient dans un carré central de 330 px,
  donc il survit au masque comme au rendu 48 px de la liste des applis.
- **Image de présentation** : Play peut recadrer et surimprimer selon la surface,
  tout le contenu reste donc au centre, à plus de 200 px des bords.
