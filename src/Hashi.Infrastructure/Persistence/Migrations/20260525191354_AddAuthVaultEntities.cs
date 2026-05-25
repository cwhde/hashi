using System;
using Microsoft.EntityFrameworkCore.Migrations;
using Npgsql.EntityFrameworkCore.PostgreSQL.Metadata;

#nullable disable

namespace Hashi.Infrastructure.Persistence.Migrations
{
    /// <inheritdoc />
    public partial class AddAuthVaultEntities : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AlterColumn<int>(
                name: "Id",
                table: "setup_state",
                type: "integer",
                nullable: false,
                oldClrType: typeof(int),
                oldType: "integer")
                .Annotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn);

            migrationBuilder.AlterColumn<int>(
                name: "Id",
                table: "app_settings",
                type: "integer",
                nullable: false,
                oldClrType: typeof(int),
                oldType: "integer")
                .Annotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn);

            migrationBuilder.CreateTable(
                name: "passkey_credentials",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    CredentialId = table.Column<byte[]>(type: "bytea", nullable: false),
                    PublicKey = table.Column<byte[]>(type: "bytea", nullable: false),
                    SignCount = table.Column<long>(type: "bigint", nullable: false),
                    Nickname = table.Column<string>(type: "character varying(128)", maxLength: 128, nullable: false),
                    PrfSupported = table.Column<bool>(type: "boolean", nullable: false),
                    PrfSalt = table.Column<byte[]>(type: "bytea", nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_passkey_credentials", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "secret_records",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    Purpose = table.Column<string>(type: "character varying(64)", maxLength: 64, nullable: false),
                    Label = table.Column<string>(type: "character varying(256)", maxLength: 256, nullable: false),
                    AdminWrappedDekBlob = table.Column<byte[]>(type: "bytea", nullable: false),
                    ServiceWrappedDekBlob = table.Column<byte[]>(type: "bytea", nullable: true),
                    CiphertextBlob = table.Column<byte[]>(type: "bytea", nullable: false),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false),
                    UpdatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_secret_records", x => x.Id);
                });

            migrationBuilder.CreateTable(
                name: "vault_wrapped_keys",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "uuid", nullable: false),
                    WrapMethod = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    WrappedKeyBlob = table.Column<byte[]>(type: "bytea", nullable: false),
                    RecoveryKeyHash = table.Column<string>(type: "text", nullable: true),
                    PasskeyCredentialId = table.Column<Guid>(type: "uuid", nullable: true),
                    CreatedAtUtc = table.Column<DateTimeOffset>(type: "timestamp with time zone", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_vault_wrapped_keys", x => x.Id);
                    table.ForeignKey(
                        name: "FK_vault_wrapped_keys_passkey_credentials_PasskeyCredentialId",
                        column: x => x.PasskeyCredentialId,
                        principalTable: "passkey_credentials",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.SetNull);
                });

            migrationBuilder.CreateIndex(
                name: "IX_passkey_credentials_CredentialId",
                table: "passkey_credentials",
                column: "CredentialId",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_secret_records_Purpose",
                table: "secret_records",
                column: "Purpose");

            migrationBuilder.CreateIndex(
                name: "IX_vault_wrapped_keys_PasskeyCredentialId",
                table: "vault_wrapped_keys",
                column: "PasskeyCredentialId");

            migrationBuilder.CreateIndex(
                name: "IX_vault_wrapped_keys_WrapMethod",
                table: "vault_wrapped_keys",
                column: "WrapMethod");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "secret_records");

            migrationBuilder.DropTable(
                name: "vault_wrapped_keys");

            migrationBuilder.DropTable(
                name: "passkey_credentials");

            migrationBuilder.AlterColumn<int>(
                name: "Id",
                table: "setup_state",
                type: "integer",
                nullable: false,
                oldClrType: typeof(int),
                oldType: "integer")
                .OldAnnotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn);

            migrationBuilder.AlterColumn<int>(
                name: "Id",
                table: "app_settings",
                type: "integer",
                nullable: false,
                oldClrType: typeof(int),
                oldType: "integer")
                .OldAnnotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn);
        }
    }
}
