package music

import (
	"context"
	"errors"
	"log/slog"
	"os"
)

// ChainAlbumArtFetcher interroge plusieurs sources d'artwork dans l'ordre et
// retient la première qui reconnaît l'album.
//
// L'ordre porte une décision : MusicBrainz identifie la sortie par une requête
// PAR CHAMPS, donc il ne peut pas confondre deux albums d'un même artiste ;
// iTunes, en recherche plein-texte, retombe sur l'album le plus populaire quand
// le titre n'est pas indexé (prod 2026-07-25 : deux albums Neon Harbor avec la
// pochette de Mixed Signals). MusicBrainz passe donc d'abord, et iTunes ne sert
// qu'à couvrir les albums absents de Cover Art Archive - sans danger désormais,
// puisqu'il vérifie lui aussi la correspondance avant de renvoyer une image.
//
// `os.ErrNotExist` d'une source signifie « je ne connais pas cet album » : on
// passe à la suivante. Une autre erreur (réseau, quota) est journalisée et on
// continue aussi, pour qu'une source indisponible ne prive pas des suivantes.
type ChainAlbumArtFetcher struct {
	fetchers []AlbumArtFetcher
}

func NewChainAlbumArtFetcher(fetchers ...AlbumArtFetcher) *ChainAlbumArtFetcher {
	kept := make([]AlbumArtFetcher, 0, len(fetchers))
	for _, f := range fetchers {
		if f != nil {
			kept = append(kept, f)
		}
	}
	return &ChainAlbumArtFetcher{fetchers: kept}
}

func (c *ChainAlbumArtFetcher) Fetch(ctx context.Context, artist, album string) ([]byte, string, error) {
	if len(c.fetchers) == 0 {
		return nil, "", os.ErrNotExist
	}
	for _, f := range c.fetchers {
		// Une annulation du scan doit arrêter la chaîne, pas glisser à la source
		// suivante qui échouerait aussitôt.
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		data, mime, err := f.Fetch(ctx, artist, album)
		switch {
		case err == nil && len(data) > 0:
			return data, mime, nil
		case err == nil:
			// Succès annoncé mais corps vide : traiter comme une absence plutôt
			// que d'écrire un fichier de 0 octet en cache.
			continue
		case errors.Is(err, os.ErrNotExist):
			continue
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, "", err
		default:
			slog.WarnContext(ctx, "album art source failed, trying the next one",
				slog.String("artist", artist), slog.String("album", album), slog.Any("err", err))
		}
	}
	// Aucune source ne reconnaît l'album : l'appelant doit voir une ABSENCE
	// (os.ErrNotExist), qu'il distingue d'une panne pour retirer une éventuelle
	// image périmée du cache.
	return nil, "", os.ErrNotExist
}
