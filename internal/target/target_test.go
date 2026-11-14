package target_test

import (
	"context"
	"testing"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/gitx"
	"github.com/sakshamsaxena/blaze/internal/target"
	"github.com/sakshamsaxena/blaze/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestResolveLocal(t *testing.T) {
	fx := testutil.NewFixture(t)
	git := gitx.New(fx.Dir)

	cfg := config.Config{Base: "main", Head: "feature", Remote: "origin"}
	tgt, err := target.Resolve(context.Background(), git, cfg)
	require.NoError(t, err)
	require.Equal(t, target.KindLocal, tgt.Kind)
	require.Equal(t, fx.BaseSHA, tgt.BaseSHA)
	require.Equal(t, fx.HeadSHA, tgt.HeadSHA)
}

func TestResolveLocalNothingToReview(t *testing.T) {
	fx := testutil.NewFixture(t)
	git := gitx.New(fx.Dir)

	cfg := config.Config{Base: "feature", Head: "feature", Remote: "origin"}
	_, err := target.Resolve(context.Background(), git, cfg)
	require.ErrorContains(t, err, "nothing to review")
}
