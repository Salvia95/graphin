"""Raw-SQL reporting path — Phase 7b SQL-literal xref fixture."""


def load_active_postings(db):
    return db.execute(
        "SELECT id, title FROM job_posting WHERE status = 'open' ORDER BY id"
    )
