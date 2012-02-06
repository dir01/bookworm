from datetime import datetime

from decorator import decorator


@decorator
def print_execution_time(func, *args, **kwargs):
    begin = datetime.now()
    try:
        return func(*args, **kwargs)
    finally:
        print 'Execution time: %s' % (datetime.now() - begin)
