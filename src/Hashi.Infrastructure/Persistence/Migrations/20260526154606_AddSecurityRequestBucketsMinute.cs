using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddSecurityRequestBucketsMinute : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "security_request_buckets",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    BucketStartUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    ClientIp = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    Resource = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    TraefikInstance = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    CountryCode = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: true),
                    RegionCode = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true),
                    Asn = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: true),
                    StatusClass = table.Column<int>(type: "integer", nullable: false),
                    Method = table.Column<string>(type: "character varying(16)", maxLength: 16, nullable: false),
                    PathPrefix = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    TotalCount = table.Column<long>(type: "bigint", nullable: false),
                    AllowedCount = table.Column<long>(type: "bigint", nullable: false),
                    BlockedCount = table.Column<long>(type: "bigint", nullable: false),
                    ChallengedCount = table.Column<long>(type: "bigint", nullable: false),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_security_request_buckets", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "IX_security_request_buckets_BucketStartUtc",
                table: "security_request_buckets",
                column: "BucketStartUtc");

            migrationBuilder.CreateIndex(
                name: "IX_security_request_buckets_BucketStartUtc_ClientIp_Resource_T~",
                table: "security_request_buckets",
                columns: new[] { "BucketStartUtc", "ClientIp", "Resource", "TraefikInstance", "CountryCode", "RegionCode", "Asn", "StatusClass", "Method", "PathPrefix" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_security_request_buckets_ClientIp",
                table: "security_request_buckets",
                column: "ClientIp");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "security_request_buckets");
        }
    }
}
