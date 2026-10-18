-- Reverse 000044_lsj_sense_summary_cache. Dropping the table discards every
-- cached LSJ sense summary; the next preparation that offers the summary would
-- regenerate it. As with every down migration here, this is not assumed safe for
-- destructive production rollback.

DROP TABLE public.lsj_sense_summaries;
