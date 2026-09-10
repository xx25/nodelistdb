/*
 * patch-config -- set the handful of MBSE settings a test mailer needs,
 * without mbsetup's curses menus.
 *
 * MBSE writes $MBSE_ROOT/etc/config.data as a raw dump of struct sysconfig
 * (lib/dbcfg.c: one fread of sizeof(CFG), no header), so reading it,
 * assigning fields and writing it back is the whole job. Compiled against
 * MBSE's own header so the layout can never drift from the binaries'.
 *
 * mbtask writes every other default on first start; all that is missing for
 * an answering mailer is an AKA and the identity it announces in the
 * handshake.
 */
#include "mbselib.h"

struct sysconfig cfg;

int main(int argc, char **argv)
{
    FILE           *fp;
    char            path[PATH_MAX];
    unsigned short  zone, net, node, point;

    if (argc != 6) {
        fprintf(stderr, "usage: patch-config <zone:net/node.point> <bbs name> <sysop> <location> <emsi flags>\n");
        return 1;
    }
    if (getenv("MBSE_ROOT") == NULL) {
        fprintf(stderr, "MBSE_ROOT is not set\n");
        return 1;
    }
    point = 0;
    if (sscanf(argv[1], "%hu:%hu/%hu.%hu", &zone, &net, &node, &point) < 3) {
        fprintf(stderr, "cannot parse address %s\n", argv[1]);
        return 1;
    }

    snprintf(path, sizeof(path) - 1, "%s/etc/config.data", getenv("MBSE_ROOT"));
    if ((fp = fopen(path, "r")) == NULL) {
        fprintf(stderr, "cannot open %s (has mbtask run yet?)\n", path);
        return 1;
    }
    if (fread(&cfg, sizeof(cfg), 1, fp) != 1) {
        fprintf(stderr, "short read on %s\n", path);
        fclose(fp);
        return 1;
    }
    fclose(fp);

    cfg.aka[0].zone  = zone;
    cfg.aka[0].net   = net;
    cfg.aka[0].node  = node;
    cfg.aka[0].point = point;
    snprintf(cfg.aka[0].domain, sizeof(cfg.aka[0].domain) - 1, "testzone");
    cfg.akavalid[0] = TRUE;

    snprintf(cfg.bbs_name,   sizeof(cfg.bbs_name) - 1,   "%s", argv[2]);
    snprintf(cfg.sysop_name, sizeof(cfg.sysop_name) - 1, "%s", argv[3]);
    snprintf(cfg.location,   sizeof(cfg.location) - 1,   "%s", argv[4]);
    snprintf(cfg.IP_Flags,   sizeof(cfg.IP_Flags) - 1,   "%s", argv[5]);
    snprintf(cfg.sysop,      sizeof(cfg.sysop) - 1,      "mbse");

    /* Answer on every transport we want to exercise. */
    cfg.NoEMSI   = FALSE;
    cfg.NoWazoo  = FALSE;
    cfg.NoZmodem = FALSE;
    cfg.NoZedzap = FALSE;
    cfg.NoMD5    = FALSE;

    /* Log everything: the mailer's own log is the point of this container. */
    cfg.cico_loglevel = 0x7FFFFFFF;

    if ((fp = fopen(path, "r+")) == NULL) {
        fprintf(stderr, "cannot reopen %s for writing\n", path);
        return 1;
    }
    if (fwrite(&cfg, sizeof(cfg), 1, fp) != 1) {
        fprintf(stderr, "short write on %s\n", path);
        fclose(fp);
        return 1;
    }
    fclose(fp);

    printf("config.data: aka %s, system \"%s\", sysop \"%s\", flags %s\n",
           argv[1], argv[2], argv[3], argv[5]);
    return 0;
}
