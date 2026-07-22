package parse

// Phase 7a: code→DB reference extraction (docs/phase7-spec.md §1.3).

import (
	"testing"
)

func parseInline(t *testing.T, rel, src string) *FileResult {
	t.Helper()
	res, err := File(rel, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func wantDBRefs(t *testing.T, n *Node, want ...DBRef) {
	t.Helper()
	if n == nil {
		t.Fatal("node missing")
	}
	if len(n.DBRefs) != len(want) {
		t.Fatalf("DBRefs = %+v, want %+v", n.DBRefs, want)
	}
	for i, w := range want {
		if n.DBRefs[i] != w {
			t.Fatalf("DBRefs[%d] = %+v, want %+v", i, n.DBRefs[i], w)
		}
	}
}

// --- Java ---

func TestJavaJPAExplicitTable(t *testing.T) {
	res := parseInline(t, "src/JobPosting.java", `package com.acme;
import jakarta.persistence.*;

@Entity
@Table(name = "job_posting")
public class JobPosting {
  private Long id;
}
`)
	wantDBRefs(t, nodeByID(res, "com.acme.JobPosting"),
		DBRef{Name: "job_posting", Source: DBRefExplicit})
}

func TestJavaJPAQualifiedAnnotation(t *testing.T) {
	res := parseInline(t, "src/A.java", `package com.acme;
@jakarta.persistence.Entity
@jakarta.persistence.Table(name = "orders")
class A {}
`)
	wantDBRefs(t, nodeByID(res, "com.acme.A"),
		DBRef{Name: "orders", Source: DBRefExplicit})
}

func TestJavaJPAEntityConvention(t *testing.T) {
	res := parseInline(t, "src/JobPosting.java", `package com.acme;
@Entity
public class JobPosting {}
`)
	wantDBRefs(t, nodeByID(res, "com.acme.JobPosting"),
		DBRef{Name: "JobPosting", Source: DBRefConvention})
}

func TestJavaNoAnnotationNoDBRefs(t *testing.T) {
	res := parseInline(t, "src/Plain.java", `package com.acme;
public class Plain {}
`)
	wantDBRefs(t, nodeByID(res, "com.acme.Plain"))
}

// --- Kotlin ---

func TestKotlinJPAExplicitTable(t *testing.T) {
	res := parseInline(t, "src/JobPosting.kt", `package com.acme

@Entity
@Table(name = "job_posting")
class JobPosting(val id: Long)
`)
	wantDBRefs(t, nodeByID(res, "com.acme.JobPosting"),
		DBRef{Name: "job_posting", Source: DBRefExplicit})
}

func TestKotlinJPAEntityConvention(t *testing.T) {
	res := parseInline(t, "src/Resume.kt", `package com.acme

@Entity
class Resume(val id: Long)
`)
	wantDBRefs(t, nodeByID(res, "com.acme.Resume"),
		DBRef{Name: "Resume", Source: DBRefConvention})
}

// --- Python ---

func TestPythonTablename(t *testing.T) {
	res := parseInline(t, "app/models/order.py", `class Order(Base):
    __tablename__ = "orders"
    id = Column(Integer, primary_key=True)
`)
	wantDBRefs(t, nodeByID(res, "app.models.order.Order"),
		DBRef{Name: "orders", Source: DBRefExplicit})
}

func TestPythonDjangoMetaDBTable(t *testing.T) {
	res := parseInline(t, "app/models.py", `class Job(models.Model):
    class Meta:
        db_table = "job_posting"
`)
	wantDBRefs(t, nodeByID(res, "app.models.Job"),
		DBRef{Name: "job_posting", Source: DBRefExplicit})
	// nested Meta class carries none of its own
	wantDBRefs(t, nodeByID(res, "app.models.Job.Meta"))
}

func TestPythonPlainClassNoDBRefs(t *testing.T) {
	res := parseInline(t, "app/svc.py", `class Service:
    name = "not a table mapping"
`)
	wantDBRefs(t, nodeByID(res, "app.svc.Service"))
}

// --- TypeScript ---

func TestTSTypeORMEntityExplicit(t *testing.T) {
	res := parseInline(t, "src/order.ts", `import { Entity } from "typeorm";

@Entity("orders")
export class Order {
  id: number;
}
`)
	wantDBRefs(t, nodeByID(res, "src.order.Order"),
		DBRef{Name: "orders", Source: DBRefExplicit})
}

func TestTSTypeORMEntityOptionsObject(t *testing.T) {
	res := parseInline(t, "src/li.ts", `@Entity({ name: "line_item", schema: "sales" })
class LineItem {}
`)
	wantDBRefs(t, nodeByID(res, "src.li.LineItem"),
		DBRef{Name: "line_item", Source: DBRefExplicit})
}

func TestTSTypeORMEntityConvention(t *testing.T) {
	res := parseInline(t, "src/li.ts", `@Entity()
export class LineItem {}
`)
	wantDBRefs(t, nodeByID(res, "src.li.LineItem"),
		DBRef{Name: "LineItem", Source: DBRefConvention})
}

func TestTSPrismaClientRefs(t *testing.T) {
	res := parseInline(t, "src/svc.ts", `export class OrderService {
  constructor(private prisma: PrismaClient) {}
  async list() {
    await this.prisma.$transaction([]);
    return this.prisma.orderItem.findMany();
  }
}

export function count() {
  return prisma.company.count();
}
`)
	wantDBRefs(t, nodeByID(res, "src.svc.OrderService.list"),
		DBRef{Name: "orderItem", Source: DBRefClient})
	wantDBRefs(t, nodeByID(res, "src.svc.count"),
		DBRef{Name: "company", Source: DBRefClient})
}

func TestTSNonPrismaReceiverIgnored(t *testing.T) {
	res := parseInline(t, "src/svc.ts", `export function f(repo) {
  return repo.orders.findMany();
}
`)
	wantDBRefs(t, nodeByID(res, "src.svc.f"))
}

// --- Prisma SSOT aliases ---

func TestPrismaModelAliases(t *testing.T) {
	res := parseSSOT(t, "prisma/schema.prisma")
	users := nodeByID(res, "db.app.public.users")
	if users == nil {
		t.Fatalf("users node missing: %v", ids(res))
	}
	if len(users.Aliases) != 2 || users.Aliases[0] != "User" || users.Aliases[1] != "user" {
		t.Fatalf("users aliases = %v, want [User user]", users.Aliases)
	}
	// model Tag has no @@map: physical name == model name → lcfirst alias only
	tag := nodeByID(res, "db.app.public.Tag")
	if tag == nil {
		t.Fatalf("Tag node missing: %v", ids(res))
	}
	if len(tag.Aliases) != 1 || tag.Aliases[0] != "tag" {
		t.Fatalf("Tag aliases = %v, want [tag]", tag.Aliases)
	}
}
