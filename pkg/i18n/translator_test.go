// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Milos Vasic

package i18n_test

import (
	"context"
	"testing"

	"digital.vasic.conversation/pkg/i18n"
)

// TestNoopTranslator_T_ReturnsMsgIDVerbatim asserts that the
// stripped-down fallback Translator emits the message ID unchanged.
// Per CONST-035 / Article XI §11.9 this verbatim-fallback is itself
// positive runtime evidence — operators see exactly which key was
// resolved without a bundle.
func TestNoopTranslator_T_ReturnsMsgIDVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}
	got := tr.T(context.Background(), "conversation_compress_unknown_strategy", map[string]any{
		"strategy": "weird",
	})
	const want = "conversation_compress_unknown_strategy"
	if got != want {
		t.Fatalf("NoopTranslator.T mismatch:\n got = %q\nwant = %q", got, want)
	}
}

// TestNoopTranslator_TPlural_ReturnsMsgIDVerbatim mirrors the T
// assertion for plural-form lookups.
func TestNoopTranslator_TPlural_ReturnsMsgIDVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}
	got := tr.TPlural(context.Background(), "conversation_replay_fetch_failed", 3, nil)
	const want = "conversation_replay_fetch_failed"
	if got != want {
		t.Fatalf("NoopTranslator.TPlural mismatch:\n got = %q\nwant = %q", got, want)
	}
}

// TestNoopTranslator_T_NilArgs_ReturnsMsgIDVerbatim ensures the noop
// implementation tolerates nil arg maps without panic — important for
// call-sites that have no template substitutions (e.g. the cache-miss
// error after a replay completes).
func TestNoopTranslator_T_NilArgs_ReturnsMsgIDVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}
	got := tr.T(context.Background(), "conversation_replay_cache_miss_after_replay", nil)
	const want = "conversation_replay_cache_miss_after_replay"
	if got != want {
		t.Fatalf("NoopTranslator.T(nil args) mismatch:\n got = %q\nwant = %q", got, want)
	}
}
