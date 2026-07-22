package com.acme;

import jakarta.persistence.Entity;
import jakarta.persistence.Table;

@Entity
@Table(name = "job_posting")
public class JobPosting {

    private Long id;

    private Long companyId;

    public Long getId() {
        return id;
    }

    public Long getCompanyId() {
        return companyId;
    }
}
