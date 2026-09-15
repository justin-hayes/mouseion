ALTER TABLE deck_preparations
 ADD COLUMN cards_with_fallback_gloss integer NOT NULL DEFAULT 0;

ALTER TABLE deck_preparations
 ADD CONSTRAINT deck_preparations_cards_with_fallback_gloss_nonnegative_check
 CHECK(cards_with_fallback_gloss >= 0),
 ADD CONSTRAINT deck_preparations_cards_with_fallback_gloss_check
 CHECK(cards_with_fallback_gloss <= total_cards);
