-- graphin dbssot fixture: state-style DDL (supabase 유사 베이스라인 덤프)

CREATE TABLE IF NOT EXISTS public.company_group (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name varchar(100) NOT NULL UNIQUE
);

CREATE TABLE public.company (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES public.company_group (id),
    code varchar(50) NOT NULL UNIQUE,
    display_name varchar(100) NOT NULL
);

CREATE TABLE public.job_posting (
    id bigint GENERATED ALWAYS AS IDENTITY,
    company_id bigint NOT NULL,
    title varchar(200) NOT NULL, -- 공고 제목
    status varchar(20) NOT NULL DEFAULT 'open',
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT job_posting_pkey PRIMARY KEY (id),
    CONSTRAINT fk_job_posting_company FOREIGN KEY (company_id)
        REFERENCES public.company (id) ON DELETE CASCADE,
    CONSTRAINT chk_job_posting_status CHECK (status IN ('open', 'closed', 'draft'))
);

CREATE TABLE public.resume (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid NOT NULL,
    title varchar(120) NOT NULL
);

ALTER TABLE public.resume
    ADD CONSTRAINT fk_resume_user FOREIGN KEY (user_id)
    REFERENCES auth.users (id) ON DELETE CASCADE;

CREATE VIEW public.v_active_job_posting AS
SELECT jp.id, jp.title, c.display_name
FROM job_posting jp
JOIN company c ON c.id = jp.company_id
WHERE jp.status = 'open';

CREATE OR REPLACE FUNCTION public.tg_job_posting_set_updated_at() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_job_posting_updated_at
    BEFORE UPDATE ON public.job_posting
    FOR EACH ROW EXECUTE FUNCTION public.tg_job_posting_set_updated_at();

ALTER TABLE public.job_posting ENABLE ROW LEVEL SECURITY;

CREATE POLICY job_posting_public_read ON public.job_posting
    FOR SELECT TO anon, authenticated USING (status = 'open');

CREATE INDEX idx_job_posting_company ON public.job_posting (company_id);
