using System.Net;
using Hashi.Infrastructure.Persistence;
using Hashi.Infrastructure.Persistence.Entities;
using Microsoft.EntityFrameworkCore;

namespace Hashi.Infrastructure.Platform;

public sealed class SecuritySubjectService(HashiDbContext db, TimeProvider? timeProvider = null)
{
    public async Task<(SecuritySubjectEntity Subject, SecuritySubjectStateEntity State)> ResolveOrCreateIpAsync(
        IPAddress clientIp,
        string? countryCode,
        string? regionCode,
        string? asn,
        CancellationToken cancellationToken = default)
    {
        var normalized = SecuritySubjectNormalizer.NormalizeIp(clientIp);
        var now = timeProvider?.GetUtcNow() ?? DateTimeOffset.UtcNow;
        var subject = await db.SecuritySubjects
            .SingleOrDefaultAsync(
                x => x.SubjectType == normalized.SubjectType && x.NormalizedValue == normalized.NormalizedValue,
                cancellationToken);
        if (subject is null)
        {
            subject = new SecuritySubjectEntity
            {
                SubjectType = normalized.SubjectType,
                SubjectValue = normalized.SubjectValue,
                NormalizedValue = normalized.NormalizedValue,
                FirstSeenAtUtc = now,
                LastSeenAtUtc = now,
            };
            db.SecuritySubjects.Add(subject);
        }

        subject.LastSeenAtUtc = now;
        if (SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Country, countryCode, out var country))
        {
            subject.LastCountry = country.NormalizedValue;
        }

        if (SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Region, regionCode, out var region))
        {
            subject.LastRegion = region.NormalizedValue;
        }

        if (SecuritySubjectNormalizer.TryNormalize(SecuritySubjectTypeNames.Asn, asn, out var normalizedAsn))
        {
            subject.LastAsn = normalizedAsn.NormalizedValue;
        }

        await db.SaveChangesAsync(cancellationToken);

        var state = await db.SecuritySubjectStates
            .SingleOrDefaultAsync(x => x.SecuritySubjectId == subject.Id, cancellationToken);
        if (state is null)
        {
            state = new SecuritySubjectStateEntity
            {
                SecuritySubjectId = subject.Id,
                UpdatedAtUtc = now,
            };
            db.SecuritySubjectStates.Add(state);
            await db.SaveChangesAsync(cancellationToken);
        }

        return (subject, state);
    }
}
