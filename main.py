#!/usr/bin/env python
from functools import wraps
import argparse
from datetime import datetime
from settings import PROGRAM_NAME


def syncdb():
    from db.meta import get_session
    from settings import SQLALCHEMY_URL
    from db.models import Base
    session = get_session(SQLALCHEMY_URL)
    Base.metadata.create_all(session.bind)


def print_execution_time_on_finish_or_interrupt(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        begin = datetime.now()
        try:
            func(*args, **kwargs)
        except KeyboardInterrupt:
            print '\rInterrupted. Worked for %s' % (datetime.now() - begin)
        except Exception, e:
            print 'Unhandled error. Worked for %s' % (datetime.now() - begin)
            raise
        else:
            print 'Finished. It took %s' % (datetime.now() - begin)
    return wrapper

@print_execution_time_on_finish_or_interrupt
def do_import(args):
    syncdb()
    from directory_importer import DirectoryImporter
    directory_importer = DirectoryImporter.get_instance()
    for directory in args.directories:
        directory_importer.do_import(directory)


parser = argparse.ArgumentParser(prog=PROGRAM_NAME)
subparsers = parser.add_subparsers()
parser_import = subparsers.add_parser('import', help='foo')
parser_import.add_argument('directories', type=str, nargs='+')
parser_import.set_defaults(func=do_import)


if __name__ == '__main__':
    args = parser.parse_args()
    args.func(args)