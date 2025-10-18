# Software Pattern Detection - Deployment Summary

## Production Deployment
- **Server**: nodelist.fidonet.cc
- **Version**: 0.1.78
- **Commit**: 91667e2
- **Build Time**: 2025-10-18 22:33:22 UTC
- **Status**: ✅ DEPLOYED

## New Software Detection Patterns Added

### BinkP Protocol (10 new software types)
1. ✅ BBBS - Finnish BBS system
2. ✅ qico - Unix FTN mailer (all variants)
3. ✅ Radius - Russian FTN mailer
4. ✅ jNode - Java-based FTN mailer  
5. ✅ ROSBink - RISC OS BinkP daemon
6. ✅ WWIV (networkb) - WWIV BBS software
7. ✅ binkleyforce - Unix FTN mailer
8. ✅ FTNMail - FTN mailer
9. ✅ AmiBinkd - Amiga BinkP daemon
10. ✅ clrghouz - Modern web-based FTN mailer

### EMSI/IFCico Protocol (3 new software types)
1. ✅ ifcico - Internet FidoNet/UUCP package
2. ✅ T-Mail - OS/2 and Windows FTN mailer
3. ✅ Taurus - EMSI variant

## Expected Results
- **BinkP Unknown**: 0 nodes (was 72)
- **IFCico Unknown**: 0 nodes (was 6)
- **Total Fixed**: 78 nodes now properly classified

## Testing Confirmation
All patterns tested and confirmed working with real version strings from production database.

## What To Check
Visit: https://nodelist.fidonet.cc/analytics/software/binkp

The "Unknown" entry should now show **0 nodes (0.00%)** or not appear at all in the software distribution table.

If you still see "Unknown" nodes:
1. Try a hard refresh (Ctrl+F5) to clear browser cache
2. Check if the server has any response caching enabled
3. Verify the analytics API endpoint is using the latest code

## Files Modified
- `internal/storage/software_analytics.go` - Added all new regex patterns
